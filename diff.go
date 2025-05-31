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

// Package diff provides functions to efficiently compare two slices similar to the Unix diff
// command line tool to compares files.
//
// By default the comparison functions in this package will try to find an optimal path, but may
// fall back to a good-enough path for large files with many differences to speed up the comparison.
// Unless [Optimal] is used to disable these heuristics, the time complexity is O(N^1.5 log N) and
// the space complexity is O(N) with N = len(x) + len(y).
package diff

import (
	"znkr.io/diff/internal/myers"
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

// Edit describes a singe edit of a diff.
//
//   - For Match, X and Y are set to their respective elements.
//   - For Delete, X is set to the element of the left slice that's missing in the right one and Y is
//     set to the zero value.
//   - For Insert, Y is set to he element of the right slice that's missing in the left one and X is
//     set to the zero value.
type Edit[T any] struct {
	Op   Op
	X, Y T
}

// Hunk describes a number of consecutive edits.
type Hunk[T any] struct {
	PosX, EndX int       // Start and end position in x.
	PosY, EndY int       // Start and end position in y.
	Edits      []Edit[T] // Edits to transform x[PosX:EndX] to y[PosY:EndY]
}

// Hunks compares the contents of x and y and returns the changes necessary to convert from one to
// the other.
//
// The output is a sequence of hunks that each describe a number of consecutive edits. Hunks include
// a number of matching elements before and after the last delete or insert operation. The number of
// elements can be configured using [Context].
//
// If x and y are identical, the output has length zero.
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func Hunks[T comparable](x, y []T, opts ...Option) []Hunk[T] {
	return HunksFunc(x, y, func(a, b T) bool { return a == b }, opts...)
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
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func HunksFunc[T any](x, y []T, eq func(a, b T) bool, opts ...Option) []Hunk[T] {
	cfg := defaultConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	edits := myers.Diff(x, y, eq, cfg.myers())

	context := cfg.context // for convenience

	// State being used in the loop below.
	s, t := 0, 0         // current index into x, y
	s0, t0 := 0, 0       // start of the current in-progress hunk
	var hedits []Edit[T] // edits for the current in-progress hunk
	run := 0             // number of consecutive matches

	var hunks []Hunk[T]
	finishHunk := func() {
		h := Hunk[T]{
			PosX:  s0,
			EndX:  s,
			PosY:  t0,
			EndY:  t,
			Edits: hedits,
		}
		hunks = append(hunks, h)
		hedits = nil
	}

	// The edits slice is a bit unusual, because it contains information for both x and y. When
	// iterating over it, we need to iterate over s and t independently. That is, we can't just
	// query edits using a single index.
	for s < len(x) || t < len(y) {
		del, ins := edits[s]&myers.Delete != 0, edits[t]&myers.Insert != 0

		if del || ins {
			run = 0

			// If there are no previous edits, start a new hunk or, if there's an overlap due to
			// context, continue with the previous hunk.
			if len(hedits) == 0 {
				s0, t0 = max(0, s-context), max(0, t-context)
				s1, t1 := s0, t0 // start of missing matches (didn't collect matches before now)

				// Check if the context windows for this new hunk and the previous hunk overlap. If
				// they do, continue filling that hunk.
				if len(hunks) > 0 && hunks[len(hunks)-1].EndX >= s0 {
					prev := hunks[len(hunks)-1]
					s1, t1 = prev.EndX, prev.EndY
					s0, t0 = prev.PosX, prev.PosY
					hedits = prev.Edits
					hunks = hunks[:len(hunks)-1]
				}

				// Backfill missing matches at the beginning of a hunk.
				for u, v := s1, t1; u < s && v < t; u, v = u+1, v+1 {
					hedits = append(hedits, Edit[T]{
						Op: Match,
						X:  x[u],
						Y:  y[v],
					})
				}
			}
		}

		// Handle one of these cases per iteration. That way consecutive deletions followed by
		// insertions are grouped by edit operations instead of being interleaved.
		switch {
		case del:
			hedits = append(hedits, Edit[T]{
				Op: Delete,
				X:  x[s],
			})
			s++
		case ins:
			hedits = append(hedits, Edit[T]{
				Op: Insert,
				Y:  y[t],
			})
			t++
		default:
			// If we have a non-empty in-progress hunk and we've seen as many matches as we want
			// in a context, finish the hunk. This also resets hedits.
			if len(hedits) > 0 && run >= context {
				finishHunk()
			}
			// If we have a non-empty in-progress hunk, record a match. If we're outside a hunk,
			// we don't do anything.
			if len(hedits) > 0 {
				hedits = append(hedits, Edit[T]{
					Op: Match,
					X:  x[s],
					Y:  y[t],
				})
			}
			s++
			t++
			run++
		}
	}
	if len(hedits) > 0 {
		finishHunk()
	}
	return hunks
}

// Edits compares the contents of x and y and returns the changes necessary to convert from one to
// the other.
//
// Edits returns edits for every element in the input. If both x and y are identical, the output
// will consist of a match edit for every input element.
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func Edits[T comparable](x, y []T, opts ...Option) []Edit[T] {
	return EditsFunc(x, y, func(a, b T) bool { return a == b }, opts...)
}

// EditsFunc compares the contents of x and y using the provided equality comparison and returns the
// changes necessary to convert from one to the other.
//
// EditsFunc returns edits for every element in the input. If both x and y are identical, the output
// will consist of a match edit for every input element.
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func EditsFunc[T any](x, y []T, eq func(a, b T) bool, opts ...Option) []Edit[T] {
	cfg := defaultConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	edits := myers.Diff(x, y, eq, cfg.myers())

	var ret []Edit[T]
	for s, t := 0, 0; s < len(x) || t < len(y); {
		// Handle one of these cases per iteration. That way consecutive deletions followed by
		// insertions are grouped by edit operations instead of being interleaved.
		switch {
		case edits[s]&myers.Delete != 0:
			ret = append(ret, Edit[T]{
				Op: Delete,
				X:  x[s],
			})
			s++
		case edits[t]&myers.Insert != 0:
			ret = append(ret, Edit[T]{
				Op: Insert,
				Y:  y[t],
			})
			t++
		default:
			ret = append(ret, Edit[T]{
				Op: Match,
				X:  x[s],
				Y:  y[t],
			})
			s++
			t++
		}
	}

	return ret
}

// Option configures the behavior of comparison functions.
type Option func(*config)

// Context sets the number of matches to include as a prefix and postfix for hunks returned in
// [Hunks] and [HunksFunc]. The default is 3.
func Context(n int) Option {
	return func(cfg *config) {
		cfg.context = max(0, n)
	}
}

// Optimal finds an optimal diff irrespective of the cost. By default, the comparison functions in
// this package limit the cost for large inputs with many differences by applying heuristics that
// reduce the time complexity.
//
// With this option, the runtime is O(ND) where N = len(x) + len(y), and D is the number of
// differences between x and y.
func Optimal() Option {
	return func(cfg *config) {
		cfg.optimal = true
	}
}

type config struct {
	context int
	optimal bool
}

var defaultConfig = config{
	context: 3,
	optimal: false,
}

func (cfg *config) myers() myers.Options {
	return myers.Options{
		Optimal: cfg.optimal,
	}
}
