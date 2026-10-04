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
	"crypto/sha256"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

func TestMyersSplit(t *testing.T) {
	tests := []struct {
		inX, inY     string
		wantX, wantY string
	}{
		// The input and output of this tests are strings containing markers
		// that define ranges. For example, ab[cde]fg represents the string
		// abcdefg and the range [2, 5]. The input consists of two strings and
		// must always define a single range (the area of interest). The output
		// are two strings representing the split areas. Everything in between
		// the two splits must be identical in both output strings.
		//
		// In the diffing algorithm, the outputs ranges will be used as input
		// ranges recursively. This pattern is emulated below.
		//
		// I realize that this is a bit unconventional, but I wanted a way to
		// understand the test at a glace without looking up strings parts from
		// indices and this is the best I could come up with.
		//
		//     inX          inY          wantX         wantY
		{"[ABCABBA]", "[CBABAC]", "[ABC]AB[BA]", "[CB]AB[AC]"},
		{"[ABC]ABBA", "[CB]ABAC", "[A]B[C]ABBA", "[C]B[]ABAC"},
		{"ABCAB[BA]", "CBAB[AC]", "ABCAB[B]A[]", "CBAB[]A[C]"},
		{"[A]BCABBA", "[C]BABAC", "[][A]BCABBA", "[C][]BABAC"},
		{"AB[C]ABBA", "CB[]ABAC", "AB[C][]ABBA", "CB[][]ABAC"},

		{"[Florian]", "[Zenker]", "[F][lorian]", "[Zenke][r]"},
		{"F[lorian]", "[Zenke]r", "F[lor][ian]", "[Ze][nke]r"},
		{"F[lor]ian", "[Ze]nker", "F[l][or]ian", "[Ze][]nker"},
		{"Flor[ian]", "Ze[nke]r", "Flor[ia]n[]", "Ze[]n[ke]r"},

		{"[axxxxxxxxb]", "[cxxxxxxxxd]", "[a]xxxxxxxx[b]", "[c]xxxxxxxx[d]"},
		{"[axxxyyxxxb]", "[cxxxzzxxxd]", "[axxx][yyxxxb]", "[cxxxzz][xxxd]"},
		{"[axxx]yyxxxb", "[cxxxzz]xxxd", "[a]xxx[]yyxxxb", "[c]xxx[zz]xxxd"},
		{"axxx[yyxxxb]", "cxxxzz[xxxd]", "axxx[yy]xxx[b]", "cxxxzz[]xxx[d]"},

		// For performance and simplicity, split skips the d=0 diagonal that
		// handles matches in prefixes, suffixes and fully identical inputs.
		// These are handled at a higher level, this test only makes sure that
		// prefix and postfix are handled correctly
		{"abcdefg[0]", "abcdefg[]", "abcdefg[0][]", "abcdefg[][]"},
		{"[0]abcdefg", "[]abcdefg", "[0][]abcdefg", "[][]abcdefg"},
		{"abcd[0]efg", "abcd[]efg", "abcd[0][]efg", "abcd[][]efg"},

		// Differently sized inputs will cause the algorithm to walk over the
		// edge of the grid. The tests below test that this edge condition is
		// handled correctly.
		{"[abcdefghijklmnoparstuvzxyz]", "[x]", "[abcdefghijklm][noparstuvzxyz]", "[][x]"},
		{"[abcdefghijklmnoparstuvzxyz]", "[]", "[abcdefghijklm][noparstuvzxyz]", "[][]"},
		{"[x]", "[abcdefghijklmnoparstuvzxyz]", "[][x]", "[abcdefghijklm][noparstuvzxyz]"},
		{"[]", "[abcdefghijklmnoparstuvzxyz]", "[][]", "[abcdefghijklm][noparstuvzxyz]"},

		// We're not testing the case that both x and y are empty, because we're
		// never going to call it with an empty input.
	}

	eq := func(a, b byte) bool { return a == b }
	for _, tt := range tests {
		x, smin, smax := parseSplitInput(tt.inX)
		y, tmin, tmax := parseSplitInput(tt.inY)

		var m myers[byte]
		smin0, smax0, tmin0, tmax0 := m.init([]byte(x), []byte(y), eq)
		if smin < smin0 || smax > smax0 {
			t.Fatalf("invalid test case: s outside of valid range: [%v, %v] not in [%v, %v]", smin, smax, smin0, smax0)
		}
		if tmin < tmin0 || tmax > tmax0 {
			t.Fatalf("invalid test case: t outside of valid range: [%v, %v] not in [%v, %v]", tmin, tmax, tmin0, tmax0)
		}
		if smin == smax && tmin == tmax {
			t.Fatalf("invalid test case: both ranges are empty.")
		}
		s0, s1, t0, t1, _, _ := m.split(smin, smax, tmin, tmax, true, eq)

		gotX := renderSplitResult(x, smin, s0, s1, smax)
		gotY := renderSplitResult(y, tmin, t0, t1, tmax)
		if gotX != tt.wantX || gotY != tt.wantY {
			t.Errorf("splitting %v, %v -> %v, %v, want %v, %v", tt.inX, tt.inY, gotX, gotY, tt.wantX, tt.wantY)
		}

		if x[s0:s1] != y[t0:t1] {
			t.Errorf("splitting %v, %v resulted in inconsistent middle: %v != %v", tt.inX, tt.inY, x[s0:s1], y[t0:t1])
		}
	}
}

func TestMyersCompare_commonPrefixSuffix(t *testing.T) {
	// compare is called on pieces of the inputs that can start or end with
	// matches. The first and last elements of x and y differ, so init doesn't
	// strip anything, and compare gets the piece between them.
	tests := []struct {
		x, y string
		want int // number of edits
	}{
		{"zabz", "yacy", 2}, // common prefix "a"
		{"zbaz", "ycay", 2}, // common suffix "a"
		{"zaabz", "yaacy", 2},
		{"zabcz", "yaxcy", 2}, // common prefix "a" and suffix "c"
	}
	eq := func(a, b byte) bool { return a == b }
	for _, tt := range tests {
		x, y := []byte(tt.x), []byte(tt.y)
		var m myers[byte]
		m.init(x, y, eq)
		m.compare(1, len(x)-1, 1, len(y)-1, true, eq)
		got := 0
		for i := 1; i < len(x)-1; i++ {
			if m.rx.Get(i) {
				got++
			}
		}
		for i := 1; i < len(y)-1; i++ {
			if m.ry.Get(i) {
				got++
			}
		}
		m.release()
		if got != tt.want {
			t.Errorf("compare(%q, %q) between the first and last element = %d edits, want %d", tt.x, tt.y, got, tt.want)
		}
	}
}

func TestMyersSplit_largeRandomInputs(t *testing.T) {
	eq := func(x, y int32) bool { return x == y }
	for i := range 20 {
		seed := sha256.Sum256(fmt.Append(nil, i))
		t.Run(fmt.Sprintf("seed=%x", seed), func(t *testing.T) {
			t.Parallel()
			rng := rand.New(rand.NewChaCha8(seed))
			// must be large enough to beat the min cost limit
			x := make([]int32, 1<<16-rng.IntN(1<<10))
			for s := range x {
				x[s] = int32(rng.IntN(10))
			}
			// must be large enough to beat the min cost limit
			y := make([]int32, 1<<16-rng.IntN(1<<10))
			for t := range y {
				y[t] = int32(rng.IntN(10))
			}

			var m myers[int32]
			smin, smax, tmin, tmax := m.init(x, y, eq)
			s0, s1, t0, t1, opt0, opt1 := m.split(smin, smax, tmin, tmax, false, eq)
			if !slices.Equal(x[s0:s1], y[t0:t1]) {
				t.Errorf("splitting resulted in non-matching middle in iteration %d, [s0=%d, s1=%d, t0=%d, t1=%d, opt0=%v, opt1=%v]", i, s0, s1, t0, t1, opt0, opt1)
			}
		})
	}
}

func TestMyersSplit_largeSimilarInputs(t *testing.T) {
	eq := func(x, y int32) bool { return x == y }
	for i := range 20 {
		seed := sha256.Sum256(fmt.Append(nil, i))
		t.Run(fmt.Sprintf("seed=%x", seed), func(t *testing.T) {
			t.Parallel()
			rng := rand.New(rand.NewChaCha8(seed))
			// must be large enough to beat the min cost limit
			x := make([]int32, 1<<16-rng.IntN(1<<10))
			for s := range x {
				x[s] = int32(rng.IntN(10))
			}
			// must be large enough to beat the min cost limit
			y := make([]int32, 1<<16-rng.IntN(1<<10))
			for t := range y {
				if t%30 < 5 || t+3 >= len(x) {
					// Five lines of noise
					y[t] = int32(rng.IntN(10))
				} else {
					// 25 lines of equality
					y[t] = x[t+3]
				}
			}

			var m myers[int32]
			smin, smax, tmin, tmax := m.init(x, y, eq)
			s0, s1, t0, t1, opt0, opt1 := m.split(smin, smax, tmin, tmax, false, eq)
			if !slices.Equal(x[s0:s1], y[t0:t1]) {
				t.Errorf("splitting resulted in non-matching middle in iteration %d, [s0=%d, s1=%d, t0=%d, t1=%d, opt0=%v, opt1=%v]", i, s0, s1, t0, t1, opt0, opt1)
			}
			// The inputs share runs of 25 elements, so the GOOD_DIAGONAL
			// heuristic finds a middle of at least goodDiagMinLen.
			if s1-s0 < goodDiagMinLen {
				t.Errorf("splitting resulted in a middle of %d elements in iteration %d, want at least %d", s1-s0, i, goodDiagMinLen)
			}
		})
	}
}

func FuzzMyersSplit(f *testing.F) {
	eq := func(a, b byte) bool { return a == b }
	f.Fuzz(func(t *testing.T, x, y []byte, optimal bool) {
		var m myers[byte]
		smin, smax, tmin, tmax := m.init([]byte(x), []byte(y), eq)

		if smin == smax && tmin == tmax {
			t.Skip("invalid test case: both ranges are empty (e.g. because the inputs are identical)")
		}

		s0, s1, t0, t1, _, _ := m.split(smin, smax, tmin, tmax, optimal, eq)
		if !slices.Equal(x[s0:s1], y[t0:t1]) {
			t.Errorf("found a middle that didn't match: %q vs %q", x[s0:s1], y[t0:t1])
		}
	})
}

func parseSplitInput(in string) (out string, min, max int) {
	var sb strings.Builder
	sb.Grow(len(in) - 2)

	min, max = math.MinInt, math.MaxInt
	offs := 0
	for i, c := range in {
		switch c {
		case '[':
			if min != math.MinInt {
				panic("invalid split input spec: " + in)
			}
			min = i
			offs++
		case ']':
			if max != math.MaxInt {
				panic("invalid split input spec: " + in)
			}
			max = i - offs
			offs++
		default:
			sb.WriteRune(c)
		}
	}
	if min == math.MinInt || max == math.MaxInt {
		panic("invalid split input spec: " + in)
	}
	out = sb.String()
	return
}

func renderSplitResult(in string, min0, max0, min1, max1 int) string {
	var sb strings.Builder
	sb.Grow(len(in) + 4)

	for i := min(min0, 0); i < max(max1+1, len(in)); i++ {
		if min0 == i {
			sb.WriteRune('[')
		}
		if max0 == i {
			sb.WriteRune(']')
		}

		if min1 == i {
			sb.WriteRune('[')
		}
		if max1 == i {
			sb.WriteRune(']')
		}
		if i >= 0 && i < len(in) {
			sb.WriteByte(in[i])
		}

	}
	return sb.String()
}

// TestMyersCompare_vArrayBounds runs compare on random inputs, including
// heavily skewed ones. The v-arrays are sized for k in [-M-1, N+1], so a split
// that leaves that range fails a bounds check.
func TestMyersCompare_vArrayBounds(t *testing.T) {
	eq := func(x, y int32) bool { return x == y }
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range 200 {
		n := rng.IntN([]int{10, 100, 1000, 5000}[i%4])
		m := rng.IntN([]int{10, 100, 1000, 5000}[(i/4)%4])
		alpha := []int{2, 5, 50}[i%3]
		x := make([]int32, n)
		for s := range x {
			x[s] = int32(rng.IntN(alpha))
		}
		y := make([]int32, m)
		for t := range y {
			if t < n && rng.IntN(4) != 0 {
				y[t] = x[t]
			} else {
				y[t] = int32(rng.IntN(alpha))
			}
		}
		for _, optimal := range []bool{true, false} {
			var my myers[int32]
			smin, smax, tmin, tmax := my.init(x, y, eq)
			if got, want := len(my.vf), (smax-smin)+(tmax-tmin)+3; got != want {
				t.Fatalf("len(vf) = %d, want %d", got, want)
			}
			my.compare(smin, smax, tmin, tmax, optimal, eq)
		}
	}
}
