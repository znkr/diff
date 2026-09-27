// Copyright 2025 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package rvecs contains functions to work with the result vectors, the
// internal representation that's used by the myers algorithm and is then
// translated to a user facing API. The internal representation is separate from
// the exported representation because it needs to solve a number of different
// problems.
package rvecs

import (
	"math/rand/v2"
	"slices"
	"testing"
)

func TestMake(t *testing.T) {
	for _, n := range []int{0, 1, 62, 63, 64, 65, 127, 128, 200} {
		for _, m := range []int{0, 1, 63, 64, 65, 130} {
			rx, ry := Make(n, m)
			if rx.Len() != n+1 || ry.Len() != m+1 {
				t.Fatalf("Make(%d, %d) lengths = %d, %d, want %d, %d", n, m, rx.Len(), ry.Len(), n+1, m+1)
			}
			if rx.NextSet(0) != rx.Len() || ry.NextSet(0) != ry.Len() {
				t.Fatalf("Make(%d, %d) returned vectors with set elements", n, m)
			}
			// rx and ry share one allocation; setting every element of one
			// must not change the other.
			for i := range rx.Len() {
				rx.Set(i)
			}
			if got := ry.NextSet(0); got != ry.Len() {
				t.Fatalf("Make(%d, %d): setting rx sets ry[%d]", n, m, got)
			}
			for i := range ry.Len() {
				ry.Set(i)
			}
			if got := rx.NextClear(0); got != rx.Len() {
				t.Fatalf("Make(%d, %d): setting ry clears rx[%d]", n, m, got)
			}
			// Release puts the set words back into the pool; the next Make
			// must still return cleared vectors.
			Release(rx, ry)
		}
	}
}

func TestSetClear(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for range 100 {
		want := make([]bool, rng.IntN(300))
		v := fromBools(want)
		if v.Len() != len(want) {
			t.Fatalf("Len() = %d, want %d", v.Len(), len(want))
		}
		if len(want) == 0 {
			continue
		}
		for range 2 * len(want) {
			i := rng.IntN(len(want))
			if rng.IntN(2) == 0 {
				v.Set(i)
				want[i] = true
			} else {
				v.Clear(i)
				want[i] = false
			}
		}
		for i, b := range want {
			if got := v.Get(i); got != b {
				t.Fatalf("Get(%d) = %t, want %t (len %d)", i, got, b, len(want))
			}
		}
	}
}

func TestSetRange(t *testing.T) {
	for _, n := range []int{0, 1, 63, 64, 65, 128, 130, 200} {
		for i := 0; i <= n; i++ {
			for _, j := range []int{i, i + 1, i + 63, i + 64, i + 65, i + 130, n} {
				if j < i || j > n {
					continue
				}
				v := fromBools(make([]bool, n))
				v.SetRange(i, j)
				for k := range n {
					if want := i <= k && k < j; v.Get(k) != want {
						t.Fatalf("after SetRange(%d, %d) on length %d, Get(%d) = %t, want %t", i, j, n, k, !want, want)
					}
				}
				// Bits at and after n are never set.
				if n%64 != 0 && v.w[len(v.w)-1]>>(n%64) != 0 {
					t.Fatalf("SetRange(%d, %d) on length %d sets bits after the end", i, j, n)
				}
			}
		}
	}
}

func TestSetSorted(t *testing.T) {
	rng := rand.New(rand.NewPCG(9, 10))
	for range 200 {
		want := make([]bool, rng.IntN(300))
		p := rng.Float64()
		var idx []int
		for i := range want {
			if rng.Float64() < p {
				want[i] = true
				idx = append(idx, i)
			}
		}
		v := fromBools(make([]bool, len(want)))
		v.SetSorted(idx)
		for i, b := range want {
			if v.Get(i) != b {
				t.Fatalf("after SetSorted(%v), Get(%d) = %t, want %t", idx, i, !b, b)
			}
		}
	}
}

func TestNextPrevFull(t *testing.T) {
	// Vectors where every element is set or every element is clear, with
	// lengths around word boundaries.
	for _, n := range []int{0, 1, 63, 64, 65, 128, 129} {
		set := make([]bool, n)
		for i := range set {
			set[i] = true
		}
		none := fromBools(make([]bool, n))
		full := fromBools(set)
		for i := range n + 2 {
			want := min(i, n)
			if got := none.NextClear(i); got != want {
				t.Fatalf("NextClear(%d) on %d clear elements = %d, want %d", i, n, got, want)
			}
			if got := none.NextSet(i); got != n {
				t.Fatalf("NextSet(%d) on %d clear elements = %d, want %d", i, n, got, n)
			}
			if got := full.NextSet(i); got != want {
				t.Fatalf("NextSet(%d) on %d set elements = %d, want %d", i, n, got, want)
			}
			if got := full.NextClear(i); got != n {
				t.Fatalf("NextClear(%d) on %d set elements = %d, want %d", i, n, got, n)
			}
		}
		for i := -1; i < n; i++ {
			if got := none.PrevClear(i); got != i {
				t.Fatalf("PrevClear(%d) on %d clear elements = %d, want %d", i, n, got, i)
			}
			if got := none.PrevSet(i); got != -1 {
				t.Fatalf("PrevSet(%d) on %d clear elements = %d, want -1", i, n, got)
			}
			if got := full.PrevSet(i); got != i {
				t.Fatalf("PrevSet(%d) on %d set elements = %d, want %d", i, n, got, i)
			}
			if got := full.PrevClear(i); got != -1 {
				t.Fatalf("PrevClear(%d) on %d set elements = %d, want -1", i, n, got)
			}
		}
	}
}

func TestNextPrev(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for range 200 {
		r := make([]bool, rng.IntN(300))
		p := rng.Float64()
		for i := range r {
			r[i] = rng.Float64() < p
		}
		v := fromBools(r)
		for i, b := range r {
			if v.Get(i) != b {
				t.Fatalf("Get(%d) = %t, want %t", i, !b, b)
			}
		}
		for i := range len(r) + 1 {
			wantSet, wantClear := len(r), len(r)
			if j := slices.Index(r[i:], true); j >= 0 {
				wantSet = i + j
			}
			if j := slices.Index(r[i:], false); j >= 0 {
				wantClear = i + j
			}
			if got := v.NextSet(i); got != wantSet {
				t.Fatalf("NextSet(%d) = %d, want %d (len %d)", i, got, wantSet, len(r))
			}
			if got := v.NextClear(i); got != wantClear {
				t.Fatalf("NextClear(%d) = %d, want %d (len %d)", i, got, wantClear, len(r))
			}
		}
		for i := -1; i < len(r); i++ {
			wantSet, wantClear := -1, -1
			for j := i; j >= 0 && (wantSet < 0 || wantClear < 0); j-- {
				if r[j] && wantSet < 0 {
					wantSet = j
				}
				if !r[j] && wantClear < 0 {
					wantClear = j
				}
			}
			if got := v.PrevSet(i); got != wantSet {
				t.Fatalf("PrevSet(%d) = %d, want %d (len %d)", i, got, wantSet, len(r))
			}
			if got := v.PrevClear(i); got != wantClear {
				t.Fatalf("PrevClear(%d) = %d, want %d (len %d)", i, got, wantClear, len(r))
			}
		}
	}
}

func fromBools(r []bool) Vec {
	v := Vec{w: make([]uint64, words(len(r))), n: len(r)}
	for i, b := range r {
		if b {
			v.Set(i)
		}
	}
	return v
}
