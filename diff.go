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
//   - For Match, both X and Y contain the matching element.
//   - For Delete, X contains the deleted element and Y is unset (zero value).
//   - For Insert, Y contains the inserted element and X is unset (zero value).
type Edit[T any] struct {
	Op   Op
	X, Y T
}

// Hunk describes a sequence of consecutive edits.
type Hunk[T any] struct {
	PosX, EndX int       // Start and end position in x.
	PosY, EndY int       // Start and end position in y.
	Edits      []Edit[T] // Edits to transform x[PosX:EndX] to y[PosY:EndY]
}

// Hunks compares the contents of x and y and returns the changes necessary to convert from one to
// the other.
//
// The output is a sequence of hunks. A hunk represents a contiguous block of changes (insertions
// and deletions) along with some surrounding context. The amount of context can be configured using
// [Context].
//
// If x and y are identical, the output has length zero.
//
// The following options are supported: [diff.Context], [diff.Optimal]
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func Hunks[T comparable](x, y []T, opts ...Option) []Hunk[T] {
	cfg := config.FromOptions(opts, config.Context|config.Optimal)
	rx, ry := impl.Diff(x, y, cfg)
	return hunks(x, y, rx, ry, cfg)
}

// HunksFunc compares the contents of x and y using the provided equality comparison and returns the
// changes necessary to convert from one to the other.
//
// The output is a sequence of hunks that each describe a number of consecutive edits. Hunks include
// a number of matching elements before and after the last delete or insert operation. The number of
// elements can be configured using [Context].
//
// If x and y are identical, the output has length zero.
//
// The following options are supported: [diff.Context], [diff.Optimal]
//
// Note that this function has generally worse performance than [Hunks] for diffs with many changes.
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func HunksFunc[T any](x, y []T, eq func(a, b T) bool, opts ...Option) []Hunk[T] {
	cfg := config.FromOptions(opts, config.Context|config.Optimal)
	rx, ry := impl.DiffFunc(x, y, eq, cfg)
	return hunks(x, y, rx, ry, cfg)
}

func hunks[T any](x, y []T, rx, ry []bool, cfg config.Config) []Hunk[T] {
	// Compute the number of hunks and edits, this is relatively cheap and allows us to preallocate
	// the return values.
	var nhunks, nedits int
	for hunk := range rvecs.Hunks(rx, ry, cfg) {
		nhunks++
		nedits += hunk.Edits
	}
	if nhunks == 0 {
		return nil
	}

	eout := make([]Edit[T], 0, nedits)
	hout := make([]Hunk[T], 0, nhunks)
	for hunk := range rvecs.Hunks(rx, ry, cfg) {
		for s, t := hunk.S0, hunk.T0; s < hunk.S1 || t < hunk.T1; {
			for s < hunk.S1 && rx[s] {
				eout = append(eout, Edit[T]{
					Op: Delete,
					X:  x[s],
				})
				s++
			}
			for t < hunk.T1 && ry[t] {
				eout = append(eout, Edit[T]{
					Op: Insert,
					Y:  y[t],
				})
				t++
			}
			for s < hunk.S1 && t < hunk.T1 && !rx[s] && !ry[t] {
				eout = append(eout, Edit[T]{
					Op: Match,
					X:  x[s],
					Y:  y[t],
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

// Edits compares the contents of x and y and returns the changes necessary to convert from one to
// the other.
//
// Edits returns one edit for every element in the input slices. If x and y are identical, the
// output will consist of a match edit for every input element.
//
// The following option is supported: [diff.Optimal]
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func Edits[T comparable](x, y []T, opts ...Option) []Edit[T] {
	cfg := config.FromOptions(opts, config.Optimal)
	rx, ry := impl.Diff(x, y, cfg)
	return edits(x, y, rx, ry)
}

// EditsFunc compares the contents of x and y using the provided equality comparison and returns the
// changes necessary to convert from one to the other.
//
// EditsFunc returns edits for every element in the input. If both x and y are identical, the output
// will consist of a match edit for every input element.
//
// The following option is supported: [diff.Optimal]
//
// Note that this function has generally worse performance than [Edits] for diffs with many changes.
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func EditsFunc[T any](x, y []T, eq func(a, b T) bool, opts ...Option) []Edit[T] {
	cfg := config.FromOptions(opts, config.Optimal)
	rx, ry := impl.DiffFunc(x, y, eq, cfg)
	return edits(x, y, rx, ry)
}

func edits[T any](x, y []T, rx, ry []bool) []Edit[T] {
	// Compute the number of edits, this is relatively cheap and allows us to preallocate the return
	// value.
	n, m := len(rx)-1, len(ry)-1
	var nedits int
	for s, t := 0, 0; s < n || t < m; {
		for s < n && rx[s] {
			nedits++
			s++
		}
		for t < m && ry[t] {
			nedits++
			t++
		}
		for s < n && t < m && !rx[s] && !ry[t] {
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
		for s < n && rx[s] {
			eout = append(eout, Edit[T]{
				Op: Delete,
				X:  x[s],
			})
			s++
		}
		for t < m && ry[t] {
			eout = append(eout, Edit[T]{
				Op: Insert,
				Y:  y[t],
			})
			t++
		}
		for s < n && t < m && !rx[s] && !ry[t] {
			eout = append(eout, Edit[T]{
				Op: Match,
				X:  x[s],
				Y:  y[t],
			})
			s++
			t++
		}
	}
	return eout
}
