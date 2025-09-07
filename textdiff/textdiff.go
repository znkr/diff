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

// Package textdiff provides functions to efficiently compare text line-by-line.
//
// This package is specialized for text comparison and provides unified diff output like the Unix
// diff command. The main functions are [Hunks] for grouped changes, [Edits] for individual changes,
// and [Unified] for standard diff format output.
//
// Performance: Default complexity is O(N^1.5 log N) time and O(N) space. With [Optimal], time
// complexity becomes O(ND) where N = len(x) + len(y) and D is the number of edits.
package textdiff

import (
	"fmt"
	"slices"

	"znkr.io/diff"
	"znkr.io/diff/internal/byteview"
	"znkr.io/diff/internal/config"
	"znkr.io/diff/internal/impl"
	"znkr.io/diff/internal/indentheuristic"
	"znkr.io/diff/internal/rvecs"
)

// Edit describes a single edit of a line-by-line diff.
type Edit[T string | []byte] struct {
	Op   diff.Op // Edit operation
	Line T       // Line, including newline character (if any)
}

// Hunk describes a sequence of consecutive edits.
type Hunk[T string | []byte] struct {
	PosX, EndX int       // Start and end line in x (zero-based).
	PosY, EndY int       // Start and end line in y (zero-based).
	Edits      []Edit[T] // Edits to transform x lines PosX..EndX to y lines PosY..EndY
}

// Hunks compares the lines in x and y and returns the changes necessary to convert from one to the
// other.
//
// The output is a sequence of hunks that each describe a number of consecutive edits. Hunks include
// a number of matching elements before and after the last delete or insert operation. The number of
// elements can be configured using [Context].
//
// If x and y are identical, the output has length zero.
//
// The following options are supported: [diff.Context], [diff.Optimal], [diff.Fast],
// [textdiff.IndentHeuristic]
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func Hunks[T string | []byte](x, y T, opts ...diff.Option) []Hunk[T] {
	cfg := config.FromOptions(opts, config.Context|config.Optimal|config.Fast|config.IndentHeuristic)
	xlines, _ := byteview.SplitLines(byteview.From(x))
	ylines, _ := byteview.SplitLines(byteview.From(y))
	rx, ry := impl.Diff(xlines, ylines, cfg)
	if cfg.IndentHeuristic {
		indentheuristic.Apply(xlines, ylines, rx, ry)
	}
	return hunks[T](xlines, ylines, rx, ry, cfg)
}

func hunks[T string | []byte](x, y []byteview.ByteView, rx, ry []bool, cfg config.Config) []Hunk[T] {
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
					Op:   diff.Delete,
					Line: byteview.UnsafeAs[T](x[s]),
				})
				s++
			}
			for t < hunk.T1 && ry[t] {
				eout = append(eout, Edit[T]{
					Op:   diff.Insert,
					Line: byteview.UnsafeAs[T](y[t]),
				})
				t++
			}
			for s < hunk.S1 && t < hunk.T1 && !rx[s] && !ry[t] {
				eout = append(eout, Edit[T]{
					Op:   diff.Match,
					Line: byteview.UnsafeAs[T](x[s]),
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

// Edits compares the lines in x and y and returns the changes necessary to convert from one to the
// other.
//
// Edits returns edits for every element in the input. If x and y are identical, the output will
// consist of a match edit for every input element.
//
// The following options are supported: [diff.Optimal], [diff.Fast], [textdiff.IndentHeuristic]
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func Edits[T string | []byte](x, y T, opts ...diff.Option) []Edit[T] {
	cfg := config.FromOptions(opts, config.Optimal|config.Fast|config.IndentHeuristic)
	xlines, _ := byteview.SplitLines(byteview.From(x))
	ylines, _ := byteview.SplitLines(byteview.From(y))
	rx, ry := impl.Diff(xlines, ylines, cfg)
	if cfg.IndentHeuristic {
		indentheuristic.Apply(xlines, ylines, rx, ry)
	}
	return edits[T](xlines, ylines, rx, ry)
}

func edits[T string | []byte](x, y []byteview.ByteView, rx, ry []bool) []Edit[T] {
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
				Op:   diff.Delete,
				Line: byteview.UnsafeAs[T](x[s]),
			})
			s++
		}
		for t < m && ry[t] {
			eout = append(eout, Edit[T]{
				Op:   diff.Insert,
				Line: byteview.UnsafeAs[T](y[t]),
			})
			t++
		}
		for s < n && t < m && !rx[s] && !ry[t] {
			eout = append(eout, Edit[T]{
				Op:   diff.Match,
				Line: byteview.UnsafeAs[T](x[s]),
			})
			s++
			t++
		}
	}
	return eout
}

const (
	prefixMatch  = " "
	prefixDelete = "-"
	prefixInsert = "+"
)

const missingNewline = "\n\\ No newline at end of file\n"

// Unified compares the lines in x and y and returns the changes necessary to convert from one to
// the other in unified format.
//
// The following options are supported: [diff.Context], [diff.Optimal], [diff.Fast],
// [textdiff.IndentHeuristic]
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func Unified[T string | []byte](x, y T, opts ...diff.Option) T {
	cfg := config.FromOptions(opts, config.Context|config.Optimal|config.Fast|config.IndentHeuristic)

	xlines, xMissingNewline := byteview.SplitLines(byteview.From(x))
	ylines, yMissingNewline := byteview.SplitLines(byteview.From(y))

	rx, ry := impl.Diff(xlines, ylines, cfg)

	if cfg.IndentHeuristic {
		indentheuristic.Apply(xlines, ylines, rx, ry)
	}

	// Precompute output buffer size.
	n := 0
	for h := range rvecs.Hunks(rx, ry, cfg) {
		n += len("@@ -, +, @@\n")
		n += numDigits(h.S0+1) + numDigits(h.S1-h.S0) + numDigits(h.T0+1) + numDigits(h.T1-h.T0)
		for s, t := h.S0, h.T0; s < h.S1 || t < h.T1; {
			for s < h.S1 && rx[s] {
				n += 1 + xlines[s].Len()
				s++
			}
			for t < h.T1 && ry[t] {
				n += 1 + ylines[t].Len()
				t++
			}
			for s < h.S1 && t < h.T1 && !rx[s] && !ry[t] {
				n += 1 + xlines[s].Len()
				s++
				t++
			}
		}
	}
	if xMissingNewline >= 0 {
		n += len(missingNewline)
	}
	if yMissingNewline >= 0 {
		n += len(missingNewline)
	}

	// Format output.
	var b byteview.Builder[T]
	b.Grow(n)
	for h := range rvecs.Hunks(rx, ry, cfg) {
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", h.S0+1, h.S1-h.S0, h.T0+1, h.T1-h.T0)
		for s, t := h.S0, h.T0; s < h.S1 || t < h.T1; {
			for s < h.S1 && rx[s] {
				b.WriteString(prefixDelete)
				b.WriteByteView(xlines[s])
				if s == xMissingNewline {
					b.WriteString(missingNewline)
				}
				s++
			}
			for t < h.T1 && ry[t] {
				b.WriteString(prefixInsert)
				b.WriteByteView(ylines[t])
				if t == yMissingNewline {
					b.WriteString(missingNewline)
				}
				t++
			}
			for s < h.S1 && t < h.T1 && !rx[s] && !ry[t] {
				b.WriteString(prefixMatch)
				b.WriteByteView(xlines[s])
				if s == xMissingNewline {
					b.WriteString(missingNewline)
				}
				s++
				t++
			}
		}
	}
	return b.Build()
}

func numDigits(v int) (n int) {
	switch {
	case v < 10:
		return 1
	case v < 100:
		return 2
	case v < 1000:
		return 3
	case v < 10_000:
		return 4
	case v < 100_000:
		return 5
	default:
		for ; v > 0; v /= 10 {
			n++
		}
		return n
	}
}
