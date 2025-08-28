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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"znkr.io/diff/internal/config"
)

func TestDiff(t *testing.T) {
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
			t.Run("diff", func(t *testing.T) {
				rx, ry := Diff(tt.x, tt.y, config.Default)
				got := render(rx, ry, len(tt.x), len(tt.y))
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("Diff(...) differs [-want,+got]:\n%s", diff)
				}
			})

			t.Run("diff_with_anchoring", func(t *testing.T) {
				cfg := config.Default
				cfg.ForceAnchoringHeuristic = true
				rx, ry := Diff(tt.x, tt.y, cfg)
				got := render(rx, ry, len(tt.x), len(tt.y))
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("Diff(...) differs [-want,+got]:\n%s", diff)
				}
			})

			t.Run("diff_func", func(t *testing.T) {
				rx, ry := DiffFunc(tt.x, tt.y, func(a, b string) bool { return a == b }, config.Default)
				got := render(rx, ry, len(tt.x), len(tt.y))
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("DiffFunc(...) differs [-want,+got]:\n%s", diff)
				}
			})
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
