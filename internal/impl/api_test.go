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
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"znkr.io/diff/internal/config"
	"znkr.io/diff/internal/lines"
	"znkr.io/diff/internal/rvecs"
)

func TestDiff(t *testing.T) {
	tests := []struct {
		name string
		skip func(cfg config.Config) bool
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
			skip: func(cfg config.Config) bool {
				return cfg.Mode == config.ModeFast
			},
			x:    strings.Split("ABCABBA", ""),
			y:    strings.Split("CBABAC", ""),
			want: "DIMDMMDMI",
		},
		{
			name: "ABCABBA_to_CBABAC",
			skip: func(cfg config.Config) bool {
				return cfg.Mode != config.ModeFast
			},
			x:    strings.Split("ABCABBA", ""),
			y:    strings.Split("CBABAC", ""),
			want: "DDDDDDDIIIIII",
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
			t.Run("diff", func(t *testing.T) {
				cfg := config.Default
				if tt.skip != nil && tt.skip(cfg) {
					return
				}
				rx, ry := Diff(tt.x, tt.y, cfg)
				got := render(rx, ry, len(tt.x), len(tt.y))
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("Diff(...) differs [-want,+got]:\n%s", diff)
				}
			})

			t.Run("diff_with_anchoring", func(t *testing.T) {
				cfg := config.Default
				cfg.ForceAnchoringHeuristic = true
				if tt.skip != nil && tt.skip(cfg) {
					return
				}
				rx, ry := Diff(tt.x, tt.y, cfg)
				got := render(rx, ry, len(tt.x), len(tt.y))
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("Diff(...) differs [-want,+got]:\n%s", diff)
				}
			})

			t.Run("diff_fast", func(t *testing.T) {
				cfg := config.Default
				cfg.Mode = config.ModeFast
				if tt.skip != nil && tt.skip(cfg) {
					return
				}
				rx, ry := Diff(tt.x, tt.y, cfg)
				got := render(rx, ry, len(tt.x), len(tt.y))
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("Diff(...) differs [-want,+got]:\n%s", diff)
				}
			})

			t.Run("diff_func", func(t *testing.T) {
				cfg := config.Default
				if tt.skip != nil && tt.skip(cfg) {
					return
				}
				rx, ry := DiffFunc(tt.x, tt.y, func(a, b string) bool { return a == b }, cfg)
				got := render(rx, ry, len(tt.x), len(tt.y))
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("DiffFunc(...) differs [-want,+got]:\n%s", diff)
				}
			})
		})
	}
}

func TestDiffMatchesDiffFunc(t *testing.T) {
	// Diff assigns IDs with ==, DiffFunc compares with ==. For types where ==
	// is unusual, both must produce valid edit scripts, and in minimal mode
	// both must produce the same number of edits.
	type withPtr struct {
		p *int
		n int
	}
	rng := rand.New(rand.NewPCG(7, 8))
	nan := math.NaN()
	floats := []float64{0, math.Copysign(0, -1), 1, nan, math.Inf(1), 2.5}
	anys := []any{1, "1", 1.0, int64(1), nil, [2]int{1, 2}, struct{}{}, nan}
	ptrs := []*int{new(int), new(int), nil}
	for range 300 {
		n, m := rng.IntN(30), rng.IntN(30)
		fx, fy := make([]float64, n), make([]float64, m)
		ax, ay := make([]any, n), make([]any, m)
		px, py := make([]withPtr, n), make([]withPtr, m)
		for i := range n {
			fx[i] = floats[rng.IntN(len(floats))]
			ax[i] = anys[rng.IntN(len(anys))]
			px[i] = withPtr{ptrs[rng.IntN(len(ptrs))], rng.IntN(2)}
		}
		for i := range m {
			fy[i] = floats[rng.IntN(len(floats))]
			ay[i] = anys[rng.IntN(len(anys))]
			py[i] = withPtr{ptrs[rng.IntN(len(ptrs))], rng.IntN(2)}
		}
		checkDiffMatchesDiffFunc(t, fx, fy)
		checkDiffMatchesDiffFunc(t, ax, ay)
		checkDiffMatchesDiffFunc(t, px, py)
	}
}

func TestDiffLinesPrefixSuffix(t *testing.T) {
	// DiffLines with prefix and suffix returns the diff of the lines in
	// between, and matches for the prefix and suffix. The prefix and suffix
	// lines differ between x and y, so comparing them changes the result.
	rng := rand.New(rand.NewPCG(1, 2))
	gen := func(n int) []string {
		s := make([]string, n)
		for i := range s {
			s[i] = strings.Repeat("a", 1+rng.IntN(3))
		}
		return s
	}
	for range 500 {
		npre, nsuf := rng.IntN(5), rng.IntN(5)
		xmid, ymid := gen(rng.IntN(10)), gen(rng.IntN(10))
		// The prefix and suffix passed to DiffLines are maximal, so the middles
		// start and end with different lines.
		for len(xmid) > 0 && len(ymid) > 0 && xmid[0] == ymid[0] {
			xmid, ymid = xmid[1:], ymid[1:]
		}
		for len(xmid) > 0 && len(ymid) > 0 && xmid[len(xmid)-1] == ymid[len(ymid)-1] {
			xmid, ymid = xmid[:len(xmid)-1], ymid[:len(ymid)-1]
		}
		x := lines.Split(text(slices.Concat(fill(npre, "x"), xmid, fill(nsuf, "x"))))
		y := lines.Split(text(slices.Concat(fill(npre, "y"), ymid, fill(nsuf, "y"))))
		for _, cfg := range []config.Config{
			{Mode: config.ModeMinimal},
			{Mode: config.ModeDefault},
			{Mode: config.ModeDefault, ForceAnchoringHeuristic: true},
			{Mode: config.ModeFast},
		} {
			rx, ry := Diff(xmid, ymid, cfg)
			want := strings.Repeat("M", npre) + render(rx, ry, len(xmid), len(ymid)) + strings.Repeat("M", nsuf)
			rx, ry = DiffLines(&x, &y, npre, nsuf, cfg)
			got := render(rx, ry, x.Len(), y.Len())
			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatalf("DiffLines(%v, %v) with prefix %d and suffix %d differs [-want,+got]:\n%s", x, y, npre, nsuf, diff)
			}
		}
		x.Release()
		y.Release()
	}
}

func TestDiffLinesMatchesDiff(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	gen := func(n, alpha int) []string {
		s := make([]string, n)
		for i := range s {
			s[i] = strings.Repeat("a", rng.IntN(alpha))
		}
		return s
	}
	for range 500 {
		alpha := 1 + rng.IntN(20)
		xl, yl := gen(rng.IntN(200), alpha), gen(rng.IntN(200), alpha)
		x, y, prefix, suffix := lines.SplitPair(text(xl), text(yl))
		xb, yb, _, _ := lines.SplitPair([]byte(text(xl)), []byte(text(yl)))
		for _, cfg := range []config.Config{
			{Mode: config.ModeMinimal},
			{Mode: config.ModeDefault},
			{Mode: config.ModeDefault, ForceAnchoringHeuristic: true},
			{Mode: config.ModeFast},
		} {
			rx, ry := Diff(xl, yl, cfg)
			want := render(rx, ry, len(xl), len(yl))
			rx, ry = DiffLines(&x, &y, prefix, suffix, cfg)
			got := render(rx, ry, x.Len(), y.Len())
			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatalf("DiffLines(%q, %q) differs from Diff [-want,+got]:\n%s", xl, yl, diff)
			}
			rx, ry = DiffLines(&xb, &yb, prefix, suffix, cfg)
			got = render(rx, ry, xb.Len(), yb.Len())
			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatalf("DiffLines([]byte(%q), []byte(%q)) differs from Diff [-want,+got]:\n%s", xl, yl, diff)
			}
		}
		x.Release()
		y.Release()
		xb.Release()
		yb.Release()
	}
}

// text returns the lines in l, each followed by a newline character.
func text(l []string) string {
	var sb strings.Builder
	for _, s := range l {
		sb.WriteString(s)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func fill(n int, s string) []string {
	r := make([]string, n)
	for i := range r {
		r[i] = s
	}
	return r
}

func checkDiffMatchesDiffFunc[T comparable](t *testing.T, x, y []T) {
	t.Helper()
	eq := func(a, b T) bool { return a == b }
	for _, cfg := range []config.Config{
		{Mode: config.ModeMinimal},
		{Mode: config.ModeDefault},
		{Mode: config.ModeDefault, ForceAnchoringHeuristic: true},
		{Mode: config.ModeFast},
	} {
		rx, ry := Diff(x, y, cfg)
		got := countEdits(t, x, y, rx, ry)
		hx, hy := DiffHash(x, y, maphash.ComparableHasher[T]{}, cfg)
		for i := range len(x) {
			if rx.Get(i) != hx.Get(i) {
				t.Fatalf("DiffHash(%v, %v, %+v) differs from Diff at x[%d]", x, y, cfg, i)
			}
		}
		for j := range len(y) {
			if ry.Get(j) != hy.Get(j) {
				t.Fatalf("DiffHash(%v, %v, %+v) differs from Diff at y[%d]", x, y, cfg, j)
			}
		}
		rx, ry = DiffFunc(x, y, eq, cfg)
		want := countEdits(t, x, y, rx, ry)
		if cfg.Mode == config.ModeMinimal && got != want {
			t.Fatalf("Diff(%v, %v) has %d edits, DiffFunc has %d", x, y, got, want)
		}
	}
}

func TestDiffMinimal(t *testing.T) {
	// In minimal mode, the number of edits is the number of elements that
	// aren't part of a longest common subsequence.
	rng := rand.New(rand.NewPCG(9, 10))
	eq := func(a, b int) bool { return a == b }
	cfg := config.Config{Mode: config.ModeMinimal}
	for range 5000 {
		alphabet := 2 + rng.IntN(4)
		gen := func() []int {
			x := make([]int, rng.IntN(40))
			for i := range x {
				x[i] = rng.IntN(alphabet)
			}
			return x
		}
		x, y := gen(), gen()
		want := len(x) + len(y) - 2*lcsLen(x, y)
		rx, ry := Diff(x, y, cfg)
		if got := countEdits(t, x, y, rx, ry); got != want {
			t.Fatalf("Diff(%v, %v) has %d edits, want %d", x, y, got, want)
		}
		rx, ry = DiffFunc(x, y, eq, cfg)
		if got := countEdits(t, x, y, rx, ry); got != want {
			t.Fatalf("DiffFunc(%v, %v) has %d edits, want %d", x, y, got, want)
		}
		rx, ry = DiffHash(x, y, maphash.ComparableHasher[int]{}, cfg)
		if got := countEdits(t, x, y, rx, ry); got != want {
			t.Fatalf("DiffHash(%v, %v) has %d edits, want %d", x, y, got, want)
		}
	}
}

// lcsLen returns the length of a longest common subsequence of x and y.
func lcsLen(x, y []int) int {
	next := make([]int, len(y)+1)
	cur := make([]int, len(y)+1)
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				cur[j] = next[j+1] + 1
			} else {
				cur[j] = max(next[j], cur[j+1])
			}
		}
		next, cur = cur, next
	}
	return next[0]
}

// countEdits returns the number of edits in rx and ry. It fails the test if rx
// and ry are not an edit script from x to y.
func countEdits[T comparable](t *testing.T, x, y []T, rx, ry rvecs.Vec) int {
	t.Helper()
	edits := 0
	for i, j := 0, 0; i < len(x) || j < len(y); {
		switch {
		case i < len(x) && rx.Get(i):
			edits++
			i++
		case j < len(y) && ry.Get(j):
			edits++
			j++
		case i == len(x) || j == len(y):
			t.Fatalf("edit script from %v to %v has unmatched elements", x, y)
		case x[i] != y[j]:
			t.Fatalf("edit script from %v to %v matches x[%d] = %v with y[%d] = %v", x, y, i, x[i], j, y[j])
		default:
			i++
			j++
		}
	}
	return edits
}

func render(rx, ry rvecs.Vec, n, m int) string {
	var sb strings.Builder
	for s, t := 0, 0; s < n || t < m; {
		if rx.Get(s) {
			sb.WriteRune('D')
			s++
		} else if ry.Get(t) {
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
