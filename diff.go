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
	"znkr.io/diff/internal/config"
	"znkr.io/diff/internal/edits"
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

// Hunk describes a sequence of consecutive edits.
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
// The following options are supported: [diff.Context], [diff.Optimal]
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
// The following options are supported: [diff.Context], [diff.Optimal]
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func HunksFunc[T any](x, y []T, eq func(a, b T) bool, opts ...Option) []Hunk[T] {
	cfg := config.FromOptions(opts, config.Context|config.Optimal)

	flags := myers.Diff(x, y, eq, cfg)
	hunks, nedits := edits.Hunks(flags, len(x), len(y), cfg)

	editsPrealloc := make([]Edit[T], nedits)
	out := make([]Hunk[T], 0, len(hunks))
	for _, h := range hunks {
		oh := Hunk[T]{
			PosX:  h.S0,
			EndX:  h.S1,
			PosY:  h.T0,
			EndY:  h.T1,
			Edits: editsPrealloc[:h.Edits:h.Edits][:0],
		}
		editsPrealloc = editsPrealloc[h.Edits:]
		for s, t := h.S0, h.T0; s < h.S1 || t < h.T1; {
			switch {
			case flags[s]&edits.Delete != 0:
				oh.Edits = append(oh.Edits, Edit[T]{
					Op: Delete,
					X:  x[s],
				})
				s++
			case flags[t]&edits.Insert != 0:
				oh.Edits = append(oh.Edits, Edit[T]{
					Op: Insert,
					Y:  y[t],
				})
				t++
			default:
				oh.Edits = append(oh.Edits, Edit[T]{
					Op: Match,
					X:  x[s],
					Y:  y[t],
				})
				s++
				t++
			}
		}
		out = append(out, oh)
	}
	return out
}

// Edits compares the contents of x and y and returns the changes necessary to convert from one to
// the other.
//
// Edits returns edits for every element in the input. If both x and y are identical, the output
// will consist of a match edit for every input element.
//
// The following option is supported: [diff.Optimal]
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
// The following option is supported: [diff.Optimal]
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func EditsFunc[T any](x, y []T, eq func(a, b T) bool, opts ...Option) []Edit[T] {
	cfg := config.FromOptions(opts, config.Optimal)

	flags := myers.Diff(x, y, eq, cfg)

	var ret []Edit[T]
	for s, t := 0, 0; s < len(x) || t < len(y); {
		// Handle one of these cases per iteration. That way consecutive deletions followed by
		// insertions are grouped by edit operations instead of being interleaved.
		switch {
		case flags[s]&edits.Delete != 0:
			ret = append(ret, Edit[T]{
				Op: Delete,
				X:  x[s],
			})
			s++
		case flags[t]&edits.Insert != 0:
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
