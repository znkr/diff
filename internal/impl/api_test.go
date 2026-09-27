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
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"znkr.io/diff/internal/config"
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
		rx, ry = DiffFunc(x, y, eq, cfg)
		want := countEdits(t, x, y, rx, ry)
		if cfg.Mode == config.ModeMinimal && got != want {
			t.Fatalf("Diff(%v, %v) has %d edits, DiffFunc has %d", x, y, got, want)
		}
	}
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
