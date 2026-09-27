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

package impl

import (
	"hash/maphash"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/google/go-cmp/cmp"
	"znkr.io/diff/internal/byteview"
)

func TestIDTableCollisions(t *testing.T) {
	// All lines get the same hash, which forces every lookup through probing
	// and line comparison.
	const h = 0xdeadbeef_00000007
	x := lines("a\nb\nc\na\nd\nb\n")
	tab := newIDTable(len(x))
	var got []int
	for i := range x {
		got = append(got, tab.insert(x, i, h))
	}
	if want := []int{0, 1, 2, 0, 3, 1}; !cmp.Equal(got, want) {
		t.Errorf("insert IDs = %v, want %v", got, want)
	}
	for _, tt := range []struct {
		line string
		want int
	}{
		{"a\n", 0},
		{"d\n", 3},
		{"e\n", -1},
	} {
		if got := tab.lookup(x, byteview.From(tt.line), h); got != tt.want {
			t.Errorf("lookup(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestIDTable(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	for _, n := range []int{0, 1, 2, 7, 64, 1000} {
		for _, alpha := range []int{1, 3, 100, 10000} {
			x := make([]int, n)
			for i := range x {
				x[i] = rng.IntN(alpha)
			}
			checkIDTable(t, x, []int{-1, 0, alpha / 2, alpha - 1, alpha})
		}
	}
}

func TestIDTableEquality(t *testing.T) {
	// The table uses == semantics: NaN is different from every value including
	// itself, 0 and -0 are equal, and interface values of different dynamic
	// types are different.
	type withPtr struct {
		p *int
		n int
	}
	rng := rand.New(rand.NewPCG(7, 8))
	nan := math.NaN()
	floats := []float64{0, math.Copysign(0, -1), 1, nan, math.Inf(1), 2.5}
	anys := []any{1, "1", 1.0, int64(1), nil, [2]int{1, 2}, struct{}{}, nan}
	ptrs := []*int{new(int), new(int), nil}
	for range 100 {
		n := rng.IntN(30)
		fx, ax, px := make([]float64, n), make([]any, n), make([]withPtr, n)
		for i := range n {
			fx[i] = floats[rng.IntN(len(floats))]
			ax[i] = anys[rng.IntN(len(anys))]
			px[i] = withPtr{ptrs[rng.IntN(len(ptrs))], rng.IntN(2)}
		}
		checkIDTable(t, fx, floats)
		checkIDTable(t, ax, anys)
		checkIDTable(t, px, []withPtr{{ptrs[0], 0}, {ptrs[1], 1}, {nil, 0}})
	}
}

func TestIDTableProbing(t *testing.T) {
	// Elements with distinct tags whose hashes all start probing at the last
	// slot, so every insert and lookup after the first probes past occupied
	// slots and wraps around to slot 0.
	x := []string{"a", "b", "c", "a", "d"}
	tab := newIDTable(len(x))
	hash := func(e string) uint64 { return uint64(e[0])<<48 | tab.mask }
	var got []int
	for i, e := range x {
		got = append(got, tab.insert(x, i, hash(e)))
	}
	if want := []int{0, 1, 2, 0, 3}; !cmp.Equal(got, want) {
		t.Errorf("insert IDs = %v, want %v", got, want)
	}
	for e, want := range map[string]int{"a": 0, "c": 2, "d": 3, "e": -1} {
		if got := tab.lookup(x, e, hash(e)); got != want {
			t.Errorf("lookup(%q) = %v, want %v", e, got, want)
		}
	}
}

// checkIDTable inserts every element of x into a new idTable and compares the
// IDs and the results of looking up x and extra with a map.
func checkIDTable[T comparable](t *testing.T, x, extra []T) {
	t.Helper()
	seed := maphash.MakeSeed()
	tab := newIDTable(len(x))
	defer tab.release()
	ids := make(map[T]int)
	for i, e := range x {
		want, ok := ids[e]
		if !ok {
			want = len(ids)
			ids[e] = want
		}
		if got := tab.insert(x, i, maphash.Comparable(seed, e)); got != want {
			t.Fatalf("insert(%v) = %v, want %v", e, got, want)
		}
	}
	for _, e := range append(x[:len(x):len(x)], extra...) {
		want, ok := ids[e]
		if !ok {
			want = -1
		}
		if got := tab.lookup(x, e, maphash.Comparable(seed, e)); got != want {
			t.Fatalf("lookup(%v) = %v, want %v", e, got, want)
		}
	}
}

func lines(s string) []byteview.ByteView {
	l, _ := byteview.SplitLines(byteview.From(s))
	return l
}
