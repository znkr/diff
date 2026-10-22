// Copyright 2026 Florian Zenker (flo@znkr.io)
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

//go:build go1.27

package diff

import (
	"hash/maphash"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestHunksHash(t *testing.T) {
	for _, tt := range hunksTests {
		t.Run(tt.name, func(t *testing.T) {
			got := HunksHash(tt.x, tt.y, maphash.ComparableHasher[string]{}, tt.opts...)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("HunksHash(...) result is different [-want, +got]:\n%s", diff)
			}
		})
	}
}

func TestEditsHash(t *testing.T) {
	for _, tt := range editsTests {
		t.Run(tt.name, func(t *testing.T) {
			got := EditsHash(tt.x, tt.y, maphash.ComparableHasher[string]{})
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("EditsHash(...) result is different (-want, +got):\n%s", diff)
			}
		})
	}
}

// TestHashMatchesComparable checks that HunksHash and EditsHash with
// maphash.ComparableHasher return the same results as Hunks and Edits.
func TestHashMatchesComparable(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	gen := func() []int {
		x := make([]int, rng.IntN(80))
		for i := range x {
			x[i] = rng.IntN(12)
		}
		return x
	}
	h := maphash.ComparableHasher[int]{}
	for range 2000 {
		x, y := gen(), gen()
		for _, opts := range [][]Option{nil, {Minimal()}, {Fast()}} {
			if diff := cmp.Diff(Hunks(x, y, opts...), HunksHash(x, y, h, opts...)); diff != "" {
				t.Fatalf("HunksHash(%v, %v) differs from Hunks [-Hunks, +HunksHash]:\n%s", x, y, diff)
			}
			if diff := cmp.Diff(Edits(x, y, opts...), EditsHash(x, y, h, opts...)); diff != "" {
				t.Fatalf("EditsHash(%v, %v) differs from Edits [-Edits, +EditsHash]:\n%s", x, y, diff)
			}
		}
	}
}

// foldHasher is a Hasher[string] whose equivalence relation ignores letter
// case. If collide is set, all strings have the same hash.
type foldHasher struct{ collide bool }

func (f foldHasher) Hash(h *maphash.Hash, s string) {
	if !f.collide {
		h.WriteString(strings.ToLower(s))
	}
}

func (foldHasher) Equal(a, b string) bool { return strings.ToLower(a) == strings.ToLower(b) }

// TestHashCustom checks that HunksHash and EditsHash match elements with
// h.Equal, also if every hash collides.
func TestHashCustom(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	words := []string{"a", "A", "b", "B", "c", "C", "d"}
	gen := func() []string {
		x := make([]string, rng.IntN(60))
		for i := range x {
			x[i] = words[rng.IntN(len(words))]
		}
		return x
	}
	lower := func(x []string) []string {
		l := make([]string, len(x))
		for i, s := range x {
			l[i] = strings.ToLower(s)
		}
		return l
	}
	// pos returns the operations and positions of edits, which don't depend
	// on the case of the elements.
	pos := func(es []Edit[string]) [][3]int {
		var p [][3]int
		for _, e := range es {
			p = append(p, [3]int{int(e.Op), e.PosX, e.PosY})
		}
		return p
	}
	for range 1000 {
		x, y := gen(), gen()
		for _, h := range []foldHasher{{}, {collide: true}} {
			for _, opts := range [][]Option{nil, {Minimal()}, {Fast()}} {
				want := pos(Edits(lower(x), lower(y), opts...))
				got := EditsHash(x, y, h, opts...)
				if diff := cmp.Diff(want, pos(got)); diff != "" {
					t.Fatalf("EditsHash(%q, %q, %+v) differs from Edits on lowercase inputs [-want, +got]:\n%s", x, y, h, diff)
				}
				for _, e := range got {
					if e.Op == Match && !h.Equal(e.X, e.Y) {
						t.Fatalf("EditsHash(%q, %q, %+v) matches %q with %q", x, y, h, e.X, e.Y)
					}
				}
			}
		}
	}
}

func BenchmarkHunksHash(b *testing.B) {
	for _, s := range benchmarkSpecs {
		b.Run(s.name(), func(b *testing.B) {
			b.ReportAllocs()
			x, y := s.generate([]byte{})
			for b.Loop() {
				_ = HunksHash(x, y, maphash.ComparableHasher[int]{})
			}
		})
	}
}

func BenchmarkEditsHash(b *testing.B) {
	for _, s := range benchmarkSpecs {
		b.Run(s.name(), func(b *testing.B) {
			b.ReportAllocs()
			x, y := s.generate([]byte{})
			for b.Loop() {
				_ = EditsHash(x, y, maphash.ComparableHasher[int]{})
			}
		})
	}
}
