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
// This package is specialized for text comparison and provides unified diff
// output like the Unix diff command. The main functions are [Hunks] for grouped
// changes, [Edits] for individual changes, and [Unified] for standard diff
// format output. [AppendUnified] and [WriteUnified] write the same output to a
// buffer or an [io.Writer].
//
// Performance: Default complexity is O(N^1.5 log N) time and O(N) space. With
// [diff.Minimal], time complexity becomes O(ND) where N = len(x) + len(y) and D
// is the number of edits. With [Fast], time complexity is O(N log N).
package textdiff

import (
	"io"
	"math"
	"slices"
	"strconv"
	"unsafe"

	"znkr.io/diff"
	"znkr.io/diff/internal/config"
	"znkr.io/diff/internal/impl"
	"znkr.io/diff/internal/indentheuristic"
	"znkr.io/diff/internal/lines"
	"znkr.io/diff/internal/pool"
	"znkr.io/diff/internal/rvecs"
)

// Edit describes a single edit of a line-by-line diff.
//
//   - For Match, Line contains the matching line. LineNoX and LineNoY contain
//     the respective line numbers (zero-based) in the input.
//   - For Delete, Line contains the deleted line from x. LineNoX contains the
//     line number in x and LineNoY is -1.
//   - For Insert, Line contains the inserted line from y. LineNoY contains the
//     line number in y and LineNoX is -1.
type Edit[T string | []byte] struct {
	Op               diff.Op
	LineNoX, LineNoY int
	Line             T
}

// Hunk describes a sequence of consecutive edits.
type Hunk[T string | []byte] struct {
	// Start and end line in x (zero-based).
	LineNoX, EndLineNoX int
	// Start and end line in y (zero-based).
	LineNoY, EndLineNoY int
	// Edits to transform x lines LineNoX..EndLineNoX to y lines
	// LineNoY..EndLineNoY
	Edits []Edit[T]
}

// Hunks compares the lines in x and y and returns the changes necessary to
// convert from one to the other.
//
// The output is a sequence of hunks that each describe a number of consecutive
// edits. Hunks include a number of matching elements before and after the last
// delete or insert operation. The number of elements can be configured using
// [diff.Context].
//
// If x and y are identical, the output has length zero.
//
// The following options are supported: [diff.Context], [diff.Minimal],
// [diff.Fast], [IndentHeuristic]
//
// Important: The output is not guaranteed to be stable and may change with
// minor version upgrades. DO NOT rely on the output being stable.
func Hunks[T string | []byte](x, y T, opts ...Option) []Hunk[T] {
	cfg := config.FromOptions(opts, config.Context|config.Minimal|config.Fast|config.IndentHeuristic)
	d := diffLines(x, y, cfg)
	defer d.release()
	return hunks(&d, cfg)
}

func hunks[T string | []byte](d *lineDiff[T], cfg config.Config) []Hunk[T] {
	rx, ry := d.rx, d.ry
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
					Op:      diff.Delete,
					Line:    d.xLine(s),
					LineNoX: s,
					LineNoY: -1,
				})
				s++
			}
			for e := min(ry.NextClear(t), hunk.T1); t < e; {
				eout = append(eout, Edit[T]{
					Op:      diff.Insert,
					Line:    d.yLine(t),
					LineNoX: -1,
					LineNoY: t,
				})
				t++
			}
			for e := s + min(min(rx.NextSet(s), hunk.S1)-s, min(ry.NextSet(t), hunk.T1)-t); s < e; {
				eout = append(eout, Edit[T]{
					Op:      diff.Match,
					Line:    d.xLine(s),
					LineNoX: s,
					LineNoY: t,
				})
				s++
				t++
			}
		}
		hout = append(hout, Hunk[T]{
			LineNoX:    hunk.S0,
			EndLineNoX: hunk.S1,
			LineNoY:    hunk.T0,
			EndLineNoY: hunk.T1,
			Edits:      slices.Clip(eout),
		})
		eout = eout[len(eout):]
	}
	return hout
}

// Edits compares the lines in x and y and returns the changes necessary to
// convert from one to the other.
//
// Edits returns edits for every element in the input. If x and y are identical,
// the output will consist of a match edit for every input element.
//
// The following options are supported: [diff.Minimal], [diff.Fast],
// [IndentHeuristic]
//
// Important: The output is not guaranteed to be stable and may change with
// minor version upgrades. DO NOT rely on the output being stable.
func Edits[T string | []byte](x, y T, opts ...Option) []Edit[T] {
	cfg := config.FromOptions(opts, config.Minimal|config.Fast|config.IndentHeuristic)
	d := diffLines(x, y, cfg)
	defer d.release()
	return edits(&d)
}

func edits[T string | []byte](d *lineDiff[T]) []Edit[T] {
	rx, ry := d.rx, d.ry
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
				Op:      diff.Delete,
				Line:    d.xLine(s),
				LineNoX: s,
				LineNoY: -1,
			})
			s++
		}
		for e := min(ry.NextClear(t), m); t < e; {
			eout = append(eout, Edit[T]{
				Op:      diff.Insert,
				Line:    d.yLine(t),
				LineNoX: -1,
				LineNoY: t,
			})
			t++
		}
		for e := s + min(min(rx.NextSet(s), n)-s, min(ry.NextSet(t), m)-t); s < e; {
			eout = append(eout, Edit[T]{
				Op:      diff.Match,
				Line:    d.xLine(s),
				LineNoX: s,
				LineNoY: t,
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

// Unified compares the lines in x and y and returns the changes necessary to
// convert from one to the other in unified format.
//
// The following options are supported: [diff.Context], [diff.Minimal],
// [diff.Fast], [IndentHeuristic], [TerminalColors]
//
// Important: The output is not guaranteed to be stable and may change with
// minor version upgrades. DO NOT rely on the output being stable.
func Unified[T string | []byte](x, y T, opts ...Option) T {
	out, _ := formatUnified(nil, nil, x, y, opts)
	return unsafeFromBytes[T](out)
}

// AppendUnified is [Unified], but it appends the output to dst and returns the
// extended buffer. If x and y are identical, it returns dst.
//
// The following options are supported: [diff.Context], [diff.Minimal],
// [diff.Fast], [IndentHeuristic], [TerminalColors]
//
// Important: The output is not guaranteed to be stable and may change with
// minor version upgrades. DO NOT rely on the output being stable.
func AppendUnified[T string | []byte](dst []byte, x, y T, opts ...Option) []byte {
	out, _ := formatUnified(dst, nil, x, y, opts)
	return out
}

// WriteUnified is [Unified], but it writes the output to w. It buffers the
// output and writes it in chunks of about 32 KiB, so w doesn't need to be
// buffered. If x and y are identical, it doesn't write anything. It stops at
// the first error from w and returns it.
//
// The following options are supported: [diff.Context], [diff.Minimal],
// [diff.Fast], [IndentHeuristic], [TerminalColors]
//
// Important: The output is not guaranteed to be stable and may change with
// minor version upgrades. DO NOT rely on the output being stable.
func WriteUnified[T string | []byte](w io.Writer, x, y T, opts ...Option) error {
	out, err := formatUnified(pool.Bytes.GetUncleared(0), w, x, y, opts)
	if err == nil && len(out) > 0 {
		_, err = w.Write(out)
	}
	// An io.Writer must not retain the slice it's given, so the buffer can go
	// back to the pool.
	pool.Bytes.Put(out)
	return err
}

// flushSize is the number of buffered bytes at which WriteUnified writes to its
// writer.
const flushSize = 32 << 10

// formatUnified appends the changes necessary to convert x into y in unified
// format to dst and returns the extended buffer. If w is not nil, it writes the
// buffer to w and empties it whenever it holds at least flushSize bytes, and
// returns the first error from w.
func formatUnified[T string | []byte](dst []byte, w io.Writer, x, y T, opts []Option) ([]byte, error) {
	flushAt := math.MaxInt
	if w != nil {
		flushAt = flushSize
	}

	cfg := config.FromOptions(opts, config.Context|config.Minimal|config.Fast|config.IndentHeuristic|config.TerminalColors)

	d := diffLines(x, y, cfg)
	defer d.release()
	xlines, ylines, rx, ry := &d.x, &d.y, d.rx, d.ry
	// Only the last line of an input can be missing its newline character.
	xMissingNewline := len(x) > 0 && x[len(x)-1] != '\n'
	yMissingNewline := len(y) > 0 && y[len(y)-1] != '\n'

	var colors config.ColorConfig
	if cfg.Colors != nil {
		colors = *cfg.Colors
	}

	var hbuf [16]rvecs.Hunk
	hs := rvecs.AppendHunks(hbuf[:0], rx, ry, cfg)

	// Precompute output buffer size.
	n := 0
	for _, h := range hs {
		n += len("@@ - + @@\n")
		n += rangeLen(h.S0, h.S1) + rangeLen(h.T0, h.T1)
		n += len(colors.HunkHeader) + len(colors.Reset)
		for s, t := h.S0, h.T0; s < h.S1 || t < h.T1; {
			if s < h.S1 && rx.Get(s) {
				n += len(colors.Delete) + len(colors.Reset)
				e := min(rx.NextClear(s), h.S1)
				n += (e - s) + xlines.Size(s, e)
				s = e
			}
			if t < h.T1 && ry.Get(t) {
				n += len(colors.Insert) + len(colors.Reset)
				e := min(ry.NextClear(t), h.T1)
				n += (e - t) + ylines.Size(t, e)
				t = e
			}
			if s < h.S1 && t < h.T1 && !rx.Get(s) && !ry.Get(t) {
				n += len(colors.Match) + len(colors.Reset)
				k := min(min(rx.NextSet(s), h.S1)-s, min(ry.NextSet(t), h.T1)-t)
				n += k + xlines.Size(s, s+k)
				s += k
				t += k
			}
		}
	}
	if len(hs) > 0 {
		last := hs[len(hs)-1]
		if xMissingNewline && last.S1 == xlines.Len() {
			n += len(missingNewline)
		}
		if yMissingNewline && last.T1 == ylines.Len() {
			n += len(missingNewline)
		}
	}

	// Format output.
	b := slices.Grow(dst, min(n, flushAt))
	for _, h := range hs {
		b = append(b, colors.HunkHeader...)
		b = append(b, "@@ -"...)
		b = appendRange(b, h.S0, h.S1)
		b = append(b, " +"...)
		b = appendRange(b, h.T0, h.T1)
		b = append(b, " @@"...)
		b = append(b, colors.Reset...)
		b = append(b, '\n')
		for s, t := h.S0, h.T0; s < h.S1 || t < h.T1; {
			if s < h.S1 && rx.Get(s) {
				b = append(b, colors.Delete...)
				for e := min(rx.NextClear(s), h.S1); s < e; {
					b = append(b, prefixDelete...)
					b = append(b, xlines.Line(s)...)
					if len(b) >= flushAt {
						var err error
						if b, err = flush(w, b); err != nil {
							return b, err
						}
					}
					s++
				}
				if xMissingNewline && s == xlines.Len() {
					b = append(b, missingNewline...)
				}
				b = append(b, colors.Reset...)
			}
			if t < h.T1 && ry.Get(t) {
				b = append(b, colors.Insert...)
				for e := min(ry.NextClear(t), h.T1); t < e; {
					b = append(b, prefixInsert...)
					b = append(b, ylines.Line(t)...)
					if len(b) >= flushAt {
						var err error
						if b, err = flush(w, b); err != nil {
							return b, err
						}
					}
					t++
				}
				if yMissingNewline && t == ylines.Len() {
					b = append(b, missingNewline...)
				}
				b = append(b, colors.Reset...)
			}
			if s < h.S1 && t < h.T1 && !rx.Get(s) && !ry.Get(t) {
				b = append(b, colors.Match...)
				for e := s + min(min(rx.NextSet(s), h.S1)-s, min(ry.NextSet(t), h.T1)-t); s < e; {
					b = append(b, prefixMatch...)
					b = append(b, xlines.Line(s)...)
					if len(b) >= flushAt {
						var err error
						if b, err = flush(w, b); err != nil {
							return b, err
						}
					}
					s++
					t++
				}
				if xMissingNewline && s == xlines.Len() {
					b = append(b, missingNewline...)
				}
				b = append(b, colors.Reset...)
			}
		}
	}
	return b, nil
}

// flush writes buf to w and returns buf emptied and the error from w.
func flush(w io.Writer, buf []byte) ([]byte, error) {
	_, err := w.Write(buf)
	return buf[:0], err
}

// rangeStart returns the line number that starts the hunk range [lo, hi) in a
// hunk header. Line numbers are 1-based, except that an empty range names the
// line before it, the same as GNU diff.
func rangeStart(lo, hi int) int {
	if lo == hi {
		return lo
	}
	return lo + 1
}

// appendRange appends the hunk range [lo, hi) in a hunk header to b and returns
// the extended buffer. Like GNU diff, it leaves out the line count when the
// count is 1.
func appendRange(b []byte, lo, hi int) []byte {
	if hi-lo == 1 {
		return strconv.AppendInt(b, int64(lo+1), 10)
	}
	b = strconv.AppendInt(b, int64(rangeStart(lo, hi)), 10)
	b = append(b, ',')
	return strconv.AppendInt(b, int64(hi-lo), 10)
}

// rangeLen returns the number of bytes that [appendRange] appends for [lo, hi).
func rangeLen(lo, hi int) int {
	n := numDigits(rangeStart(lo, hi))
	if hi-lo != 1 {
		n += len(",") + numDigits(hi-lo)
	}
	return n
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

// lineDiff is the diff of the lines of two inputs.
type lineDiff[T string | []byte] struct {
	xin, yin T // inputs
	x, y     lines.Lines
	rx, ry   rvecs.Vec
}

// diffLines returns the diff of the lines of x and y. Call [lineDiff.release]
// when it's no longer needed.
func diffLines[T string | []byte](x, y T, cfg config.Config) lineDiff[T] {
	d := lineDiff[T]{xin: x, yin: y}
	var prefix, suffix int
	d.x, d.y, prefix, suffix = lines.SplitPair(x, y)
	d.rx, d.ry = impl.DiffLines(&d.x, &d.y, prefix, suffix, cfg)
	if cfg.IndentHeuristic {
		indentheuristic.Apply(&d.x, &d.y, d.rx, d.ry)
	}
	return d
}

// xLine returns line i of x.
func (d *lineDiff[T]) xLine(i int) T {
	start, end := d.x.Span(i)
	return d.xin[start:end]
}

// yLine returns line i of y.
func (d *lineDiff[T]) yLine(i int) T {
	start, end := d.y.Span(i)
	return d.yin[start:end]
}

// release returns the memory of d to the pools. d must not be used after
// release.
func (d *lineDiff[T]) release() {
	d.x.Release()
	d.y.Release()
	rvecs.Release(d.rx, d.ry)
}

// unsafeFromBytes returns b as a T without copying. If T is string, b must not
// be modified afterwards.
func unsafeFromBytes[T string | []byte](b []byte) T {
	switch any((*T)(nil)).(type) {
	case *string:
		return T(unsafe.String(unsafe.SliceData(b), len(b)))
	case *[]byte:
		return T(b)
	}
	panic("never reached")
}
