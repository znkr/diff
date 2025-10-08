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
						{Insert, -1, 0, "", "foo"},
						{Insert, -1, 1, "", "bar"},
						{Insert, -1, 2, "", "baz"},
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
						{Delete, 0, -1, "foo", ""},
						{Delete, 1, -1, "bar", ""},
						{Delete, 2, -1, "baz", ""},
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
						{Match, 0, 0, "foo", "foo"},
						{Delete, 1, -1, "bar", ""},
						{Insert, -1, 1, "", "baz"},
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
						{Delete, 0, -1, "foo", ""},
						{Insert, -1, 0, "", "loo"},
						{Match, 1, 1, "bar", "bar"},
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
						{Delete, 0, -1, "A", ""},
						{Insert, -1, 0, "", "C"},
						{Match, 1, 1, "B", "B"},
						{Delete, 2, -1, "C", ""},
						{Match, 3, 2, "A", "A"},
						{Match, 4, 3, "B", "B"},
						{Delete, 5, -1, "B", ""},
						{Match, 6, 4, "A", "A"},
						{Insert, -1, 5, "", "C"},
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
						{Delete, 0, -1, "A", ""},
						{Insert, -1, 0, "", "C"},
					},
				},
				{
					PosX: 2,
					PosY: 2,
					EndX: 3,
					EndY: 2,
					Edits: []Edit[string]{
						{Delete, 2, -1, "C", ""},
					},
				},
				{
					PosX: 5,
					PosY: 4,
					EndX: 6,
					EndY: 4,
					Edits: []Edit[string]{
						{Delete, 5, -1, "B", ""},
					},
				},
				{
					PosX: 7,
					PosY: 5,
					EndX: 7,
					EndY: 6,
					Edits: []Edit[string]{
						{Insert, -1, 5, "", "C"},
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
						{Insert, -1, 0, "", "this is a new paragraph"},
						{Insert, -1, 1, "", "that is inserted at the top"},
						{Insert, -1, 2, "", ""},
						{Match, 0, 3, "this paragraph", "this paragraph"},
						{Match, 1, 4, "is not", "is not"},
						{Match, 2, 5, "changed and", "changed and"},
					},
				},
				{
					PosX: 4,
					EndX: 11,
					PosY: 7,
					EndY: 10,
					Edits: []Edit[string]{
						{Match, 4, 7, "enough to", "enough to"},
						{Match, 5, 8, "create a", "create a"},
						{Match, 6, 9, "new hunk", "new hunk"},
						{Delete, 7, -1, "", ""},
						{Delete, 8, -1, "this paragraph", ""},
						{Delete, 9, -1, "is going to be", ""},
						{Delete, 10, -1, "removed", ""},
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
						{Insert, -1, 0, "", "this is a new paragraph"},
						{Insert, -1, 1, "", "that is inserted at the top"},
						{Insert, -1, 2, "", ""},
						{Match, 0, 3, "this paragraph", "this paragraph"},
						{Match, 1, 4, "stays but is", "stays but is"},
						{Match, 2, 5, "not long enough", "not long enough"},
						{Match, 3, 6, "to create a", "to create a"},
						{Match, 4, 7, "new hunk", "new hunk"},
						{Delete, 5, -1, "", ""},
						{Delete, 6, -1, "this paragraph", ""},
						{Delete, 7, -1, "is going to be", ""},
						{Delete, 8, -1, "removed", ""},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			{
				got := Hunks(tt.x, tt.y, tt.opts...)
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("Hunks(...) result is different [-want, +got]:\n%s", diff)
				}
			}
			{
				got := HunksFunc(tt.x, tt.y, func(a, b string) bool { return a == b }, tt.opts...)
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("HunksFunc(...) result is different [-want, +got]:\n%s", diff)
				}
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
				{Match, 0, 0, "foo", "foo"},
				{Match, 1, 1, "bar", "bar"},
				{Match, 2, 2, "baz", "baz"},
			},
		},
		{
			name: "empty",
		},
		{
			name: "x-empty",
			y:    []string{"foo", "bar", "baz"},
			want: []Edit[string]{
				{Insert, -1, 0, "", "foo"},
				{Insert, -1, 1, "", "bar"},
				{Insert, -1, 2, "", "baz"},
			},
		},
		{
			name: "y-empty",
			x:    []string{"foo", "bar", "baz"},
			want: []Edit[string]{
				{Delete, 0, -1, "foo", ""},
				{Delete, 1, -1, "bar", ""},
				{Delete, 2, -1, "baz", ""},
			},
		},
		{
			name: "ABCABBA_to_CBABAC",
			x:    strings.Split("ABCABBA", ""),
			y:    strings.Split("CBABAC", ""),
			want: []Edit[string]{
				{Delete, 0, -1, "A", ""},
				{Insert, -1, 0, "", "C"},
				{Match, 1, 1, "B", "B"},
				{Delete, 2, -1, "C", ""},
				{Match, 3, 2, "A", "A"},
				{Match, 4, 3, "B", "B"},
				{Delete, 5, -1, "B", ""},
				{Match, 6, 4, "A", "A"},
				{Insert, -1, 5, "", "C"},
			},
		},
		{
			name: "same-prefix",
			x:    []string{"foo", "bar"},
			y:    []string{"foo", "baz"},
			want: []Edit[string]{
				{Match, 0, 0, "foo", "foo"},
				{Delete, 1, -1, "bar", ""},
				{Insert, -1, 1, "", "baz"},
			},
		},
		{
			name: "same-suffix",
			x:    []string{"foo", "bar"},
			y:    []string{"loo", "bar"},
			want: []Edit[string]{
				{Delete, 0, -1, "foo", ""},
				{Insert, -1, 0, "", "loo"},
				{Match, 1, 1, "bar", "bar"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			{
				got := Edits(tt.x, tt.y)
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("Edits(...) result is different (-want, +got):\n%s", diff)
				}
			}
			{
				got := EditsFunc(tt.x, tt.y, func(a, b string) bool { return a == b })
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("EditsFunc(...) result is different (-want, +got):\n%s", diff)
				}
			}
		})
	}
}

func BenchmarkHunks(b *testing.B) {
	for _, s := range benchmarkSpecs {
		b.Run(s.name(), func(b *testing.B) {
			b.ReportAllocs()
			x, y := s.generate([]byte{})
			for b.Loop() {
				_ = Hunks(x, y)
			}
		})
	}
}

func BenchmarkHunksFunc(b *testing.B) {
	for _, s := range benchmarkSpecs {
		b.Run(s.name(), func(b *testing.B) {
			b.ReportAllocs()
			x, y := s.generate([]byte{})
			for b.Loop() {
				_ = HunksFunc(x, y, func(a, b int) bool { return a == b })
			}
		})
	}
}

func BenchmarkEdits(b *testing.B) {
	for _, s := range benchmarkSpecs {
		b.Run(s.name(), func(b *testing.B) {
			b.ReportAllocs()
			x, y := s.generate([]byte{})
			for b.Loop() {
				_ = Edits(x, y)
			}
		})
	}
}

func BenchmarkEditsFunc(b *testing.B) {
	for _, s := range benchmarkSpecs {
		b.Run(s.name(), func(b *testing.B) {
			b.ReportAllocs()
			x, y := s.generate([]byte{})
			for b.Loop() {
				_ = EditsFunc(x, y, func(a, b int) bool { return a == b })
			}
		})
	}
}

type spec struct {
	N, M int // Length of x and y respectively
	D    int // Number of edits (besides edits due to size differences)
}

var benchmarkSpecs = []spec{
	{50, 50, 10},
	{500, 50, 10},
	{50, 500, 10},
	{500, 500, 10},
	{500, 500, 100},
	{5000, 5500, 100},
}

func (s spec) name() string {
	return fmt.Sprintf("N=%d_M=%d_D=%d", s.N, s.M, s.D)
}

func (s spec) generate(seed []byte) (x, y []int) {
	rng := rand.New(rand.NewChaCha8(sha256.Sum256(seed)))

	// Construct inputs based on the N, M, D specification.
	flipped := false
	n, m := s.N, s.M
	if n < m {
		n, m = m, n
		flipped = true
	}

	x = make([]int, n)
	for i := range x {
		x[i] = rng.IntN(100)
	}

	y = make([]int, m)
	delta := 0
	if n != m {
		delta = rng.IntN((n - m) / 2)
	}
	for i := range y {
		y[i] = x[i+delta]
	}

	// We might already have some changes due to the different sizes for N and M, add D
	// additional changes.
	for d := s.D; d > 0; {
		i := rng.IntN(len(y))
		if y[i] >= 0 {
			y[i] = -y[i]
			d--
		}
	}

	if flipped {
		x, y = y, x
	}
	return
}
