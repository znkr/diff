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

// Package textdiff provides functions to efficiently compare text line by line.
package textdiff

import (
	"fmt"

	"znkr.io/diff"
	"znkr.io/diff/internal/byteview"
	"znkr.io/diff/internal/config"
	"znkr.io/diff/internal/edits"
	"znkr.io/diff/internal/indentheuristic"
	"znkr.io/diff/internal/myers"
)

const (
	prefixMatch  = " "
	prefixDelete = "-"
	prefixInsert = "+"
)

const missingNewline = "\n\\ No newline at end of file\n"

// Unified compares the lines in x and y and returns the changes necessary to convert from one to
// the other in unified format.
//
// The following options are supported: [diff.Context], [diff.Optimal], [textdiff.IndentHeuristic]
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func Unified[T string | []byte](x, y T, opts ...diff.Option) T {
	cfg := config.FromOptions(opts, config.Context|config.Optimal|config.IndentHeuristic)

	xlines, xMissingNewline := byteview.SplitLines(byteview.From(x))
	ylines, yMissingNewline := byteview.SplitLines(byteview.From(y))

	rx, ry := myers.Diff(xlines, ylines, byteview.Equal, cfg)

	if cfg.IndentHeuristic {
		indentheuristic.Apply(xlines, ylines, rx, ry)
	}

	// Format output
	var b byteview.Builder[T]
	for h := range edits.Hunks(rx, ry, cfg) {
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
