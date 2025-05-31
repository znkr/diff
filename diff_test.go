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

package diff

import (
	"crypto/sha256"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestHunks(t *testing.T) {
	tests := []struct {
		name string
		x, y []string
		opts []Option
		want []Hunk[string]
	}{
		{
			name: "identical",
			x:    []string{"foo", "bar", "baz"},
			y:    []string{"foo", "bar", "baz"},
			want: nil,
		},
		{
			name: "empty",
			x:    nil,
			y:    nil,
			want: nil,
		},
		{
			name: "x-empty",
			x:    nil,
			y:    []string{"foo", "bar", "baz"},
			want: []Hunk[string]{
				{
					PosX: 0,
					PosY: 0,
					EndX: 0,
					EndY: 3,
					Edits: []Edit[string]{
						{Insert, "", "foo"},
						{Insert, "", "bar"},
						{Insert, "", "baz"},
					},
				},
			},
		},
		{
			name: "y-empty",
			x:    []string{"foo", "bar", "baz"},
			y:    nil,
			want: []Hunk[string]{
				{
					PosX: 0,
					PosY: 0,
					EndX: 3,
					EndY: 0,
					Edits: []Edit[string]{
						{Delete, "foo", ""},
						{Delete, "bar", ""},
						{Delete, "baz", ""},
					},
				},
			},
		},
		{
			name: "same-prefix",
			x:    []string{"foo", "bar"},
			y:    []string{"foo", "baz"},
			want: []Hunk[string]{
				{
					PosX: 0,
					EndX: 2,
					PosY: 0,
					EndY: 2,
					Edits: []Edit[string]{
						{Match, "foo", "foo"},
						{Delete, "bar", ""},
						{Insert, "", "baz"},
					},
				},
			},
		},
		{
			name: "same-suffix",
			x:    []string{"foo", "bar"},
			y:    []string{"loo", "bar"},
			want: []Hunk[string]{
				{
					PosX: 0,
					EndX: 2,
					PosY: 0,
					EndY: 2,
					Edits: []Edit[string]{
						{Delete, "foo", ""},
						{Insert, "", "loo"},
						{Match, "bar", "bar"},
					},
				},
			},
		},
		{
			name: "ABCABBA_to_CBABAC",
			x:    strings.Split("ABCABBA", ""),
			y:    strings.Split("CBABAC", ""),
			want: []Hunk[string]{
				{
					PosX: 0,
					PosY: 0,
					EndX: 7,
					EndY: 6,
					Edits: []Edit[string]{
						{Delete, "A", ""},
						{Insert, "", "C"},
						{Match, "B", "B"},
						{Delete, "C", ""},
						{Match, "A", "A"},
						{Match, "B", "B"},
						{Delete, "B", ""},
						{Match, "A", "A"},
						{Insert, "", "C"},
					},
				},
			},
		},
		{
			name: "ABCABBA_to_CBABAC_no_context",
			x:    strings.Split("ABCABBA", ""),
			y:    strings.Split("CBABAC", ""),
			opts: []Option{Context(0)},
			want: []Hunk[string]{
				{
					PosX: 0,
					PosY: 0,
					EndX: 1,
					EndY: 1,
					Edits: []Edit[string]{
						{Delete, "A", ""},
						{Insert, "", "C"},
					},
				},
				{
					PosX: 2,
					PosY: 2,
					EndX: 3,
					EndY: 2,
					Edits: []Edit[string]{
						{Delete, "C", ""},
					},
				},
				{
					PosX: 5,
					PosY: 4,
					EndX: 6,
					EndY: 4,
					Edits: []Edit[string]{
						{Delete, "B", ""},
					},
				},
				{
					PosX: 7,
					PosY: 5,
					EndX: 7,
					EndY: 6,
					Edits: []Edit[string]{
						{Insert, "", "C"},
					},
				},
			},
		},
		{
			name: "two-hunks",
			x: []string{
				"this paragraph",
				"is not",
				"changed and",
				"barely long",
				"enough to",
				"create a",
				"new hunk",
				"",
				"this paragraph",
				"is going to be",
				"removed",
			},
			y: []string{
				"this is a new paragraph",
				"that is inserted at the top",
				"",
				"this paragraph",
				"is not",
				"changed and",
				"barely long",
				"enough to",
				"create a",
				"new hunk",
			},
			want: []Hunk[string]{
				{
					PosX: 0,
					EndX: 3,
					PosY: 0,
					EndY: 6,
					Edits: []Edit[string]{
						{Insert, "", "this is a new paragraph"},
						{Insert, "", "that is inserted at the top"},
						{Insert, "", ""},
						{Match, "this paragraph", "this paragraph"},
						{Match, "is not", "is not"},
						{Match, "changed and", "changed and"},
					},
				},
				{
					PosX: 4,
					EndX: 11,
					PosY: 7,
					EndY: 10,
					Edits: []Edit[string]{
						{Match, "enough to", "enough to"},
						{Match, "create a", "create a"},
						{Match, "new hunk", "new hunk"},
						{Delete, "", ""},
						{Delete, "this paragraph", ""},
						{Delete, "is going to be", ""},
						{Delete, "removed", ""},
					},
				},
			},
		},
		{
			name: "overlapping-consecutive-hunks-are-merged",
			x: []string{
				"this paragraph",
				"stays but is",
				"not long enough",
				"to create a",
				"new hunk",
				"",
				"this paragraph",
				"is going to be",
				"removed",
			},
			y: []string{
				"this is a new paragraph",
				"that is inserted at the top",
				"",
				"this paragraph",
				"stays but is",
				"not long enough",
				"to create a",
				"new hunk",
			},
			want: []Hunk[string]{
				{
					PosX: 0,
					EndX: 9,
					PosY: 0,
					EndY: 8,
					Edits: []Edit[string]{
						{Insert, "", "this is a new paragraph"},
						{Insert, "", "that is inserted at the top"},
						{Insert, "", ""},
						{Match, "this paragraph", "this paragraph"},
						{Match, "stays but is", "stays but is"},
						{Match, "not long enough", "not long enough"},
						{Match, "to create a", "to create a"},
						{Match, "new hunk", "new hunk"},
						{Delete, "", ""},
						{Delete, "this paragraph", ""},
						{Delete, "is going to be", ""},
						{Delete, "removed", ""},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Hunks(tt.x, tt.y, tt.opts...)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Diff result is different [-want, +got]:\n%s", diff)
			}
		})
	}
}

func TestEdits(t *testing.T) {
	tests := []struct {
		name string
		x, y []string
		want []Edit[string]
	}{
		{
			name: "identical",
			x:    []string{"foo", "bar", "baz"},
			y:    []string{"foo", "bar", "baz"},
			want: []Edit[string]{
				{Match, "foo", "foo"},
				{Match, "bar", "bar"},
				{Match, "baz", "baz"},
			},
		},
		{
			name: "empty",
		},
		{
			name: "x-empty",
			y:    []string{"foo", "bar", "baz"},
			want: []Edit[string]{
				{Insert, "", "foo"},
				{Insert, "", "bar"},
				{Insert, "", "baz"},
			},
		},
		{
			name: "y-empty",
			x:    []string{"foo", "bar", "baz"},
			want: []Edit[string]{
				{Delete, "foo", ""},
				{Delete, "bar", ""},
				{Delete, "baz", ""},
			},
		},
		{
			name: "ABCABBA_to_CBABAC",
			x:    strings.Split("ABCABBA", ""),
			y:    strings.Split("CBABAC", ""),
			want: []Edit[string]{
				{Delete, "A", ""},
				{Insert, "", "C"},
				{Match, "B", "B"},
				{Delete, "C", ""},
				{Match, "A", "A"},
				{Match, "B", "B"},
				{Delete, "B", ""},
				{Match, "A", "A"},
				{Insert, "", "C"},
			},
		},
		{
			name: "same-prefix",
			x:    []string{"foo", "bar"},
			y:    []string{"foo", "baz"},
			want: []Edit[string]{
				{Match, "foo", "foo"},
				{Delete, "bar", ""},
				{Insert, "", "baz"},
			},
		},
		{
			name: "same-suffix",
			x:    []string{"foo", "bar"},
			y:    []string{"loo", "bar"},
			want: []Edit[string]{
				{Delete, "foo", ""},
				{Insert, "", "loo"},
				{Match, "bar", "bar"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Edits(tt.x, tt.y)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Diff result is different (-want, +got):\n%s", diff)
			}
		})
	}
}

func BenchmarkHunks(b *testing.B) {
	params := []struct {
		N, M int // Length of x and y respectively
		D    int // Number of edits (besides edits due to size differences)
	}{
		{50, 50, 10},
		{500, 50, 10},
		{50, 500, 10},
		{500, 500, 10},
		{500, 500, 100},
		{5000, 5500, 100},
	}

	for _, p := range params {
		name := fmt.Sprintf("N=%d_M=%d_D=%d", p.N, p.M, p.D)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()

			rng := rand.New(rand.NewChaCha8(sha256.Sum256([]byte(name))))

			// Construct inputs based on the N, M, D specification.
			flipped := false
			n, m := p.N, p.M
			if n < m {
				n, m = m, n
				flipped = true
			}

			x := make([]int, n)
			for i := range x {
				x[i] = rng.IntN(100)
			}

			y := make([]int, m)
			delta := 0
			if n != m {
				delta = rng.IntN((n - m) / 2)
			}
			for i := range y {
				y[i] = x[i+delta]
			}

			// We might already have some changes due to the different sizes for N and M, add D
			// additional changes.
			for d := p.D; d > 0; {
				i := rng.IntN(len(y))
				if y[i] >= 0 {
					y[i] = -y[i]
					d--
				}
			}

			if flipped {
				x, y = y, x
			}

			for b.Loop() {
				_ = Hunks(x, y)
			}
		})
	}
}
