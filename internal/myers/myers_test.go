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

package myers

import (
	"crypto/sha256"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"znkr.io/diff/internal/config"
)

func TestMyersDiff(t *testing.T) {
	tests := []struct {
		name string
		x, y []string
		want string
	}{
		{
			name: "identical",
			x:    []string{"foo", "bar", "baz"},
			y:    []string{"foo", "bar", "baz"},
			want: "MMM",
		},
		{
			name: "empty",
			x:    nil,
			y:    nil,
			want: "",
		},
		{
			name: "x-empty",
			x:    nil,
			y:    []string{"foo", "bar", "baz"},
			want: "III",
		},
		{
			name: "y-empty",
			x:    []string{"foo", "bar", "baz"},
			y:    nil,
			want: "DDD",
		},
		{
			name: "ABCABBA_to_CBABAC",
			x:    strings.Split("ABCABBA", ""),
			y:    strings.Split("CBABAC", ""),
			want: "DIMDMMDMI",
		},
		{
			name: "same-prefix",
			x:    []string{"foo", "bar"},
			y:    []string{"foo", "baz"},
			want: "MDI",
		},
		{
			name: "same-suffix",
			x:    []string{"foo", "bar"},
			y:    []string{"loo", "bar"},
			want: "DIM",
		},
		{
			name: "largish",
			x:    strings.Split("xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaay", ""),
			y:    strings.Split("waaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaait", ""),
			want: "DIMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMDII",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			{
				rx, ry := Diff(tt.x, tt.y, config.Default)
				got := render(rx, ry, len(tt.x), len(tt.y))
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("Diff(...) differs [-want,+got]:\n%s", diff)
				}
			}
			{
				rx, ry := DiffFunc(tt.x, tt.y, func(a, b string) bool { return a == b }, config.Default)
				got := render(rx, ry, len(tt.x), len(tt.y))
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("DiffFunc(...) differs [-want,+got]:\n%s", diff)
				}
			}
		})
	}
}

func render(rx, ry []bool, n, m int) string {
	var sb strings.Builder
	for s, t := 0, 0; s < n || t < m; {
		if rx[s] {
			sb.WriteRune('D')
			s++
		} else if ry[t] {
			sb.WriteRune('I')
			t++
		} else {
			sb.WriteRune('M')
			s++
			t++
		}
	}
	return sb.String()
}

func TestMyersSplit(t *testing.T) {
	tests := []struct {
		inX, inY     string
		wantX, wantY string
	}{
		// The input and output of this tests are strings containing markers that define ranges. For
		// example, ab[cde]fg represents the string abcdefg and the range [2, 5]. The input consists
		// of two strings and must always define a single range (the area of interest). The output
		// are two strings representing the split areas. Everything in between the two splits must
		// be identical in both output strings.
		//
		// In the diffing algorithm, the outputs ranges will be used as input ranges recursively.
		// This pattern is emulated below.
		//
		// I realize that this is a bit unconventional, but I wanted a way to understand the test
		// at a glace without looking up strings parts from indices and this is the best I could
		// come up with.
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

		// For performance and simplicity, split skips the d=0 diagonal that handles matches in
		// prefixes, suffixes and fully identical inputs. These are handled at a higher level,
		// this test only makes sure that prefix and postfix are handled correctly
		{"abcdefg[0]", "abcdefg[]", "abcdefg[0][]", "abcdefg[][]"},
		{"[0]abcdefg", "[]abcdefg", "[0][]abcdefg", "[][]abcdefg"},
		{"abcd[0]efg", "abcd[]efg", "abcd[0][]efg", "abcd[][]efg"},

		// Differently sized inputs will cause the algorithm to walk over the edge of the grid. The
		// tests below test that this edge condition is handled correctly.
		{"[abcdefghijklmnoparstuvzxyz]", "[x]", "[abcdefghijklm][noparstuvzxyz]", "[][x]"},
		{"[abcdefghijklmnoparstuvzxyz]", "[]", "[abcdefghijklm][noparstuvzxyz]", "[][]"},
		{"[x]", "[abcdefghijklmnoparstuvzxyz]", "[][x]", "[abcdefghijklm][noparstuvzxyz]"},
		{"[]", "[abcdefghijklmnoparstuvzxyz]", "[][]", "[abcdefghijklm][noparstuvzxyz]"},

		// We're not testing the case that both x and y are empty, because we're never going to
		// call it with an empty input.
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

func TestMyersSplit_largeInputs(t *testing.T) {
	eq := func(x, y int32) bool { return x == y }
	for i := range 20 {
		seed := sha256.Sum256(fmt.Append(nil, i))
		t.Run(fmt.Sprintf("seed=%x", seed), func(t *testing.T) {
			t.Parallel()
			rng := rand.New(rand.NewChaCha8(seed))
			x := make([]int32, 1<<16-rng.IntN(1<<10)) // must be large enough to beat the min cost limit
			for s := range x {
				x[s] = int32(rng.IntN(10))
			}
			y := make([]int32, 1<<16-rng.IntN(1<<10)) // must be large enough to beat the min cost limit
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
