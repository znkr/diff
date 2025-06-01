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
	"bytes"
	"fmt"
	"unsafe"

	"znkr.io/diff"
	"znkr.io/diff/internal/config"
	"znkr.io/diff/internal/edits"
	"znkr.io/diff/internal/myers"
)

const (
	prefixMatch  = " "
	prefixDelete = "-"
	prefixInsert = "+"
)

// Unified compares the lines in x and y and returns the changes necessary to convert from one to
// the other in unified format.
//
// The following options are supported: [diff.Context], [diff.Optimal]
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func Unified(x, y string, opts ...diff.Option) string {
	// This hackery let's us support both string and []byte types with the same implementation
	// without copying the inputs in or the outputs out. It's save because we never modify the
	// inputs or retain the output anywhere.
	xp, yp := unsafe.StringData(x), unsafe.StringData(y)
	out := UnifiedBytes(unsafe.Slice(xp, len(x)), unsafe.Slice(yp, len(y)), opts)
	return unsafe.String(unsafe.SliceData(out), len(out))
}

// UnifiedBytes compares the lines in x and y and returns the changes necessary to convert from one
// to the other in unified format.
//
// The following options are supported: [diff.Context], [diff.Optimal]
//
// Important: The output is not guaranteed to be stable and may change with minor version upgrades.
// DO NOT rely on the output being stable.
func UnifiedBytes(x, y []byte, opts []diff.Option) []byte {
	cfg := config.FromOptions(opts, config.Context|config.Optimal)

	xlines := bytes.SplitAfter(x, []byte{'\n'})
	ylines := bytes.SplitAfter(y, []byte{'\n'})

	// SplitAfter adds an empty element after the last '\n', we need to remove it because it doesn't
	// count as a line for diffs.
	if len(xlines[len(xlines)-1]) == 0 {
		xlines = xlines[:len(xlines)-1]
	}
	if len(ylines[len(ylines)-1]) == 0 {
		ylines = ylines[:len(ylines)-1]
	}

	flags := myers.Diff(xlines, ylines, bytes.Equal, cfg)
	hunks := edits.Hunks(flags, len(xlines), len(ylines), cfg)
	if len(hunks) == 0 {
		return nil
	}

	// Format output
	var b bytes.Buffer
	for i, h := range hunks {
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", h.S0+1, h.S1-h.S0, h.T0+1, h.T1-h.T0)
		for s, t := h.S0, h.T0; s < h.S1 || t < h.T1; {
			var prefix string
			var line []byte
			switch {
			case flags[s]&edits.Delete != 0:
				prefix = prefixDelete
				line = xlines[s]
				s++
			case flags[t]&edits.Insert != 0:
				prefix = prefixInsert
				line = ylines[t]
				t++
			default:
				prefix = prefixMatch
				line = xlines[s]
				s++
				t++
			}
			b.WriteString(prefix)
			b.Write(line)
			if i == len(hunks)-1 && (s == h.S1 || t == h.T1) && line[len(line)-1] != '\n' {
				b.WriteString("\n\\ No newline at end of file\n")
			}
		}
	}
	return b.Bytes()
}
