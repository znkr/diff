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

// Package lines splits inputs into lines and represents each line by its
// boundaries: offsets into the input.
package lines

import (
	"math/bits"
	"strings"
	"unsafe"

	"znkr.io/diff/internal/pool"
)

// Lines holds an input and its line boundaries. Each line includes its newline
// character.
type Lines struct {
	// s is the input, without a copy if it's a []byte. Lines holds a string
	// instead of having a type parameter for the input, because generic code
	// converts a []byte with a type switch on every call, and the diff reads
	// every line several times.
	s string
	b []int // line i is s[b[i]:b[i+1]]

	// b[lo:hi+1] are computed. SplitPair doesn't compute the boundaries of the
	// common prefix and suffix, because splitting them is most of the work for
	// a large input with few changes. extend computes them when a line is read:
	// context lines of hunks, lines the indent heuristic compares (a change can
	// slide any distance into the prefix or suffix), and every line in Edits.
	lo, hi int
}

// Split returns the lines of s. The result must only be used while s is not
// modified. Call [Lines.Release] when the result is no longer needed.
func Split[T string | []byte](s T) Lines { return split(unsafeString(s), 0, 0, 0, 0) }

// SplitPair returns the lines of x and y and the number of lines in their
// longest common prefix and suffix. The prefix and suffix don't overlap. The
// results must only be used while x and y are not modified. Call
// [Lines.Release] on both results when they are no longer needed.
func SplitPair[T string | []byte](x, y T) (xl, yl Lines, prefix, suffix int) {
	xs, ys := unsafeString(x), unsafeString(y)

	// Comparing bytes finds the common prefix and suffix without splitting
	// their lines. They consist of whole lines in both x and y, and they are
	// the longest such prefix and suffix.
	p := commonPrefix(xs, ys)
	p = strings.LastIndexByte(xs[:p], '\n') + 1
	q := commonSuffix(xs[p:], ys[p:])
	if q > 0 && !(lineStart(xs, len(xs)-q) && lineStart(ys, len(ys)-q)) {
		// The suffix starts in the middle of a line in x or y. Both have the
		// same bytes after the next newline, so starting after it is a line
		// start in both.
		if i := strings.IndexByte(xs[len(xs)-q:], '\n'); i < 0 {
			q = 0
		} else {
			q -= i + 1
		}
	}
	prefix = strings.Count(xs[:p], "\n")
	suffix = countLines(xs[len(xs)-q:])
	return split(xs, prefix, suffix, p, q), split(ys, prefix, suffix, p, q), prefix, suffix
}

// split returns the lines of s. The first npre lines end at byte p and the last
// nsuf lines start q bytes before the end of s. Only the boundaries of the
// lines between them are computed.
func split(s string, npre, nsuf, p, q int) Lines {
	mid := s[p : len(s)-q]
	n := npre + countLines(mid) + nsuf
	// b[npre] is the start of mid and b[n-nsuf] is its end. lineStarts writes
	// the boundaries between them, plus one element after the last newline in
	// mid, so b has one element of slack. That element and a newline at the end
	// of mid both land at or after b[n-nsuf], which is set after lineStarts.
	b := pool.Ints.GetUncleared(n + 2)
	b[npre] = p
	lineStarts(mid, p, b[npre+1:])
	b[n-nsuf] = len(s) - q
	return Lines{s: s, b: b[:n+1], lo: npre, hi: n - nsuf}
}

// lineStarts stores off plus the offset after each newline in s in starts, and
// then one more value. starts must have room for the number of newlines in s
// plus one.
func lineStarts(s string, off int, starts []int) {
	k, i := 0, 0
	// Two words per iteration halve the loop overhead. More don't help.
	for ; i+16 <= len(s); i += 16 {
		w := s[i : i+16]
		m0, m1 := newlineMask(w[:8]), newlineMask(w[8:])
		k = storeStarts(starts, k, off+i, m0)
		k = storeStarts(starts, k, off+i+8, m1)
	}
	for ; i+8 <= len(s); i += 8 {
		k = storeStarts(starts, k, off+i, newlineMask(s[i:i+8]))
	}
	for ; i < len(s); i++ {
		if s[i] == '\n' {
			starts[k] = off + i + 1
			k++
		}
	}
}

// newlineMask reads the 8 bytes of w as a little-endian word and returns a
// mask with the high bit set in each byte that is a newline.
func newlineMask(w string) uint64 {
	const lo7 = 0x7f7f7f7f7f7f7f7f
	const newlines = 0x0a0a0a0a0a0a0a0a
	_ = w[7]
	v := uint64(w[0]) | uint64(w[1])<<8 | uint64(w[2])<<16 | uint64(w[3])<<24 |
		uint64(w[4])<<32 | uint64(w[5])<<40 | uint64(w[6])<<48 | uint64(w[7])<<56

	// After the xor, newline bytes are zero. For each byte, (v&lo7 + lo7) sets
	// the high bit if any of the low 7 bits is set, without carrying into the
	// next byte; or-ing v adds the byte's own high bit, and or-ing lo7 sets the
	// low bits. The complement has the high bit set exactly in the bytes that
	// are newlines, and no other bits.
	v ^= newlines
	return ^((v&lo7 + lo7) | v | lo7)
}

// storeStarts stores pos plus the offset after each newline in the word that
// m is the [newlineMask] of in starts, starting at starts[k], and returns the
// index after the last one. It may also write starts[k] if m is zero.
func storeStarts(starts []int, k, pos int, m uint64) int {
	// Most words contain no newline or one. storeStarts stores the position of
	// the first newline even if there is none, and advances k only if there is
	// one: (m | -m) >> 63 is 1 for any nonzero m. If m is zero, the stored
	// value is junk that the next store overwrites. A branch on whether m is
	// zero is mispredicted about once per line, which costs more than the
	// store. Only words with more than one newline take the loop.
	starts[k] = pos + bits.TrailingZeros64(m)/8 + 1
	k += int((m | -m) >> 63)
	for m &= m - 1; m != 0; m &= m - 1 {
		starts[k] = pos + bits.TrailingZeros64(m)/8 + 1
		k++
	}
	return k
}

// Release returns the memory of l to a pool. The caller must not use l after
// Release.
func (l *Lines) Release() { pool.Ints.Put(l.b) }

// Len returns the number of lines.
func (l *Lines) Len() int { return len(l.b) - 1 }

// Line returns line i. The result shares memory with the input of l.
func (l *Lines) Line(i int) string {
	if i < l.lo || i >= l.hi {
		l.extend(i, i+1)
	}
	return l.s[l.b[i]:l.b[i+1]]
}

// At returns line i like [Lines.Line], but without computing its boundaries, so
// that it inlines. Line i must be one whose boundaries are always computed: any
// line of [Split], or a line between the common prefix and suffix of
// [SplitPair].
func (l *Lines) At(i int) string { return l.s[l.b[i]:l.b[i+1]] }

// Span returns the offsets of the first byte of line i and of the byte after it
// in the input.
func (l *Lines) Span(i int) (start, end int) {
	if i < l.lo || i >= l.hi {
		l.extend(i, i+1)
	}
	return l.b[i], l.b[i+1]
}

// Size returns the number of bytes in lines [i, j).
func (l *Lines) Size(i, j int) int {
	if i < l.lo || j > l.hi {
		l.extend(i, j)
	}
	return l.b[j] - l.b[i]
}

// extend computes the boundaries b[i:j+1] and all boundaries between them and
// the computed ones.
func (l *Lines) extend(i, j int) {
	_, _ = l.b[i], l.b[j] // panic if out of range
	for i < l.lo {
		// The lines before lo are in the common prefix, so line lo-1 ends with
		// a newline.
		l.b[l.lo-1] = strings.LastIndexByte(l.s[:l.b[l.lo]-1], '\n') + 1
		l.lo--
	}
	for j > l.hi {
		m := strings.IndexByte(l.s[l.b[l.hi]:], '\n') + 1
		if m == 0 {
			m = len(l.s) - l.b[l.hi] // last line without a newline
		}
		l.b[l.hi+1] = l.b[l.hi] + m
		l.hi++
	}
}

// countLines returns the number of lines in s. The last line may be missing a
// newline character.
func countLines(s string) int {
	n := strings.Count(s, "\n")
	if len(s) > 0 && s[len(s)-1] != '\n' {
		n++
	}
	return n
}

// lineStart reports whether a line starts at s[i].
func lineStart(s string, i int) bool { return i == 0 || s[i-1] == '\n' }

// unsafeString returns v as a string without copying. The result must only be
// read while v is not modified.
func unsafeString[T string | []byte](v T) string {
	switch v := any(v).(type) {
	case string:
		return v
	case []byte:
		return unsafe.String(unsafe.SliceData(v), len(v))
	}
	panic("never reached")
}

// commonPrefix returns the length of the common prefix of a and b.
func commonPrefix(a, b string) int {
	n := min(len(a), len(b))
	i := 0
	// Compare blocks first, which uses the runtime's vectorized comparison.
	const block = 256
	for i+block <= n && a[i:i+block] == b[i:i+block] {
		i += block
	}
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

// commonSuffix returns the length of the common suffix of a and b.
func commonSuffix(a, b string) int {
	n := min(len(a), len(b))
	i := 0
	const block = 256
	for i+block <= n && a[len(a)-i-block:len(a)-i] == b[len(b)-i-block:len(b)-i] {
		i += block
	}
	for i < n && a[len(a)-i-1] == b[len(b)-i-1] {
		i++
	}
	return i
}
