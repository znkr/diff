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
	"slices"

	"znkr.io/diff/internal/config"
	"znkr.io/diff/internal/impl"
	"znkr.io/diff/internal/rvecs"
)

// Op describes an edit operation.
//
//go:generate go tool golang.org/x/tools/cmd/stringer -type=Op
type Op int

const (
	Match  Op = iota // Two slice elements match
	Delete           // A deletion from an element on the left slice
	Insert           // An insertion of an element from the right side
)

// Edit describes a single edit of a diff.
//
//   - For Match, both X and Y contain the matching element. PosX and PosY
//     contain their respective positions in the input.
//   - For Delete, X contains the deleted element and Y is unset (zero value).
//     PosX contains its position in the input and PosY is -1.
//   - For Insert, Y contains the inserted element and X is unset (zero value).
//     PosY contains its position in the input and PosX is -1.
type Edit[T any] struct {
	Op         Op
	PosX, PosY int
	X, Y       T
}

// Hunk describes a sequence of consecutive edits.
type Hunk[T any] struct {
	PosX, EndX int       // Start and end position in x.
	PosY, EndY int       // Start and end position in y.
	Edits      []Edit[T] // Edits to transform x[PosX:EndX] to y[PosY:EndY]
}

// Hunks compares the contents of x and y and returns the changes necessary to
// convert from one to the other.
//
// The output is a sequence of hunks. A hunk represents a contiguous block of
// changes (insertions and deletions) along with some surrounding context. The
// amount of context can be configured using [Context].
//
// If x and y are identical, the output has length zero.
//
// The following options are supported: [Context], [Minimal], [Fast]
//
// Important: The output is not guaranteed to be stable and may change with
// minor version upgrades. DO NOT rely on the output being stable.
func Hunks[T comparable](x, y []T, opts ...Option) []Hunk[T] {
	cfg := config.FromOptions(opts, config.Context|config.Minimal|config.Fast)
	rx, ry := impl.Diff(x, y, cfg)
	defer rvecs.Release(rx, ry)
	return hunks(x, y, rx, ry, cfg)
}

// HunksFunc compares the contents of x and y using the provided equality
// comparison and returns the changes necessary to convert from one to the
// other.
//
// The output is a sequence of hunks that each describe a number of consecutive
// edits. Hunks include a number of matching elements before and after the last
// delete or insert operation. The number of elements can be configured using
// [Context].
//
// If x and y are identical, the output has length zero.
//
// The following options are supported: [Context], [Minimal]
//
// Note that this function has generally worse performance than [Hunks] for
// diffs with many changes.
//
// Important: The output is not guaranteed to be stable and may change with
// minor version upgrades. DO NOT rely on the output being stable.
func HunksFunc[T any](x, y []T, eq func(a, b T) bool, opts ...Option) []Hunk[T] {
	cfg := config.FromOptions(opts, config.Context|config.Minimal)
	rx, ry := impl.DiffFunc(x, y, eq, cfg)
	defer rvecs.Release(rx, ry)
	return hunks(x, y, rx, ry, cfg)
}

func hunks[T any](x, y []T, rx, ry rvecs.Vec, cfg config.Config) []Hunk[T] {
	var buf [16]rvecs.Hunk
	hs := rvecs.AppendHunks(buf[:0], rx, ry, cfg)
	if len(hs) == 0 {
		return nil
	}
	// Count the edits to preallocate the return values.
	nedits := 0
	for _, hunk := range hs {
		nedits += hunk.Edits
	}

	eout := make([]Edit[T], 0, nedits)
	hout := make([]Hunk[T], 0, len(hs))
	for _, hunk := range hs {
		for s, t := hunk.S0, hunk.T0; s < hunk.S1 || t < hunk.T1; {
			for e := min(rx.NextClear(s), hunk.S1); s < e; {
				eout = append(eout, Edit[T]{
					Op:   Delete,
					X:    x[s],
					PosX: s,
					PosY: -1,
				})
				s++
			}
			for e := min(ry.NextClear(t), hunk.T1); t < e; {
				eout = append(eout, Edit[T]{
					Op:   Insert,
					Y:    y[t],
					PosX: -1,
					PosY: t,
				})
				t++
			}
			for e := s + min(min(rx.NextSet(s), hunk.S1)-s, min(ry.NextSet(t), hunk.T1)-t); s < e; {
				eout = append(eout, Edit[T]{
					Op:   Match,
					X:    x[s],
					Y:    y[t],
					PosX: s,
					PosY: t,
				})
				s++
				t++
			}
		}
		hout = append(hout, Hunk[T]{
			PosX:  hunk.S0,
			EndX:  hunk.S1,
			PosY:  hunk.T0,
			EndY:  hunk.T1,
			Edits: slices.Clip(eout),
		})
		eout = eout[len(eout):]
	}
	return hout
}

// Edits compares the contents of x and y and returns the changes necessary to
// convert from one to the other.
//
// Edits returns one edit for every element in the input slices. If x and y are
// identical, the output will consist of a match edit for every input element.
//
// The following option is supported: [Minimal], [Fast]
//
// Important: The output is not guaranteed to be stable and may change with
// minor version upgrades. DO NOT rely on the output being stable.
func Edits[T comparable](x, y []T, opts ...Option) []Edit[T] {
	cfg := config.FromOptions(opts, config.Minimal|config.Fast)
	rx, ry := impl.Diff(x, y, cfg)
	defer rvecs.Release(rx, ry)
	return edits(x, y, rx, ry)
}

// EditsFunc compares the contents of x and y using the provided equality
// comparison and returns the changes necessary to convert from one to the
// other.
//
// EditsFunc returns edits for every element in the input. If both x and y are
// identical, the output will consist of a match edit for every input element.
//
// The following option is supported: [Minimal]
//
// Note that this function has generally worse performance than [Edits] for
// diffs with many changes.
//
// Important: The output is not guaranteed to be stable and may change with
// minor version upgrades. DO NOT rely on the output being stable.
func EditsFunc[T any](x, y []T, eq func(a, b T) bool, opts ...Option) []Edit[T] {
	cfg := config.FromOptions(opts, config.Minimal)
	rx, ry := impl.DiffFunc(x, y, eq, cfg)
	defer rvecs.Release(rx, ry)
	return edits(x, y, rx, ry)
}

func edits[T any](x, y []T, rx, ry rvecs.Vec) []Edit[T] {
	// Compute the number of edits, this is relatively cheap and allows us to
	// preallocate the return value.
	n, m := rx.Len()-1, ry.Len()-1
	var nedits int
	for s, t := 0, 0; s < n || t < m; {
		for e := min(rx.NextClear(s), n); s < e; {
			nedits++
			s++
		}
		for e := min(ry.NextClear(t), m); t < e; {
			nedits++
			t++
		}
		for e := s + min(min(rx.NextSet(s), n)-s, min(ry.NextSet(t), m)-t); s < e; {
			nedits++
			s++
			t++
		}
	}
	if nedits == 0 {
		return nil
	}

	eout := make([]Edit[T], 0, nedits)
	for s, t := 0, 0; s < n || t < m; {
		for e := min(rx.NextClear(s), n); s < e; {
			eout = append(eout, Edit[T]{
				Op:   Delete,
				X:    x[s],
				PosX: s,
				PosY: -1,
			})
			s++
		}
		for e := min(ry.NextClear(t), m); t < e; {
			eout = append(eout, Edit[T]{
				Op:   Insert,
				Y:    y[t],
				PosX: -1,
				PosY: t,
			})
			t++
		}
		for e := s + min(min(rx.NextSet(s), n)-s, min(ry.NextSet(t), m)-t); s < e; {
			eout = append(eout, Edit[T]{
				Op:   Match,
				X:    x[s],
				Y:    y[t],
				PosX: s,
				PosY: t,
			})
			s++
			t++
		}
	}
	return eout
}
