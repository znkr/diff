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

package textdiff

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"znkr.io/diff"
	"znkr.io/diff/internal/mem"
)

// Header is the file header of a unified diff.
type Header struct {
	X, Y string // Labels after "--- " and "+++ ", unquoted.
}

// ParseUnified returns the header and hunks of a unified diff, as produced by
// [Unified], git diff, GNU diff -u or BSD diff -u. The header is nil if patch
// has none. An empty patch returns nil hunks.
//
// The patch holds the diff of a single file. Text before the first file header
// or hunk header is skipped, and so is the signature that git format-patch
// writes after the last hunk. A label in double quotes is unquoted with C-style
// escapes, and text after a tab in a label, such as a timestamp, is dropped.
//
// The Line of each edit is a subslice of patch without the line prefix. It
// ends in a newline, unless a "\ No newline at end of file" line follows it.
//
// ParseUnified returns an error that names the line of the patch if the patch
// is malformed, if hunks overlap, if the patch holds more than one file, a
// binary diff, a git diff without a content change or text after the last
// hunk, or if a line starts with an ANSI escape sequence.
func ParseUnified[T string | []byte](patch T) (*Header, []Hunk[T], error) {
	var h *Header
	s := mem.Of(patch)
	if len(s) > 0 && s[len(s)-1] != '\n' {
		return nil, nil, parseError(s.Count("\n")+1, "missing newline at end of patch")
	}

	const (
		preamble    = iota // Before the first file header or hunk.
		gitHeader          // After a "diff --git" line and only git extended headers.
		gitPreamble        // After a "diff --git" line and other text.
		headerY            // After a "--- " line, which starts a file header if a "+++ " line follows.
		fileHeader         // After the file header, before the first hunk.
		hunkHeader         // After a hunk.
		body               // In a hunk body.
		signature          // After the "-- " line that starts the signature of git format-patch.
	)
	state := preamble
	var (
		labelX     mem.UnsafeString // The "--- " line of the file header.
		hunks      []Hunk[T]        // Completed hunks.
		hk         Hunk[T]          // The current hunk.
		edits      []Edit[T]        // Edits of the current hunk, with room for the edits of later hunks.
		x, y       int              // Line numbers of the next body line in x and y.
		last       *Edit[T]         // The edit of the previous line, if it is a body line.
		endX, endY bool             // Report whether the last line of x or y has been read.
	)
	lineCount := s.Count("\n")
	n, end := 0, 0
	for l := range s.Lines() {
		n++
		start := end
		end += len(l)
		if l[0] == '\x1b' {
			return nil, nil, parseError(n, "ANSI escape sequence")
		}
		if state == headerY {
			if l.HasPrefix("+++ ") {
				h = &Header{
					X: parseLabel(labelX[len("--- ") : len(labelX)-1]),
					Y: parseLabel(l[len("+++ ") : len(l)-1]),
				}
				state = fileHeader
				continue
			}
			state = preamble
		}
		switch state {
		case preamble:
			switch {
			case l.HasPrefix("diff --git "):
				state = gitHeader
				continue
			case l.HasPrefix("Binary files "):
				return nil, nil, parseError(n, "binary diff")
			}
		case gitHeader:
			switch {
			case isGitExtendedHeader(l):
				continue
			case l.HasPrefix("Binary files "), l == "GIT binary patch\n":
				return nil, nil, parseError(n, "binary diff")
			}
			state = gitPreamble
			fallthrough
		case gitPreamble:
			if l.HasPrefix("diff --git ") {
				return nil, nil, parseError(n, "patch contains more than one file")
			}
		}
		if l[0] == '\\' && (state == body || state == hunkHeader) {
			if last == nil {
				return nil, nil, parseError(n, `"\" line does not follow a body line`)
			}
			if len(last.Line) == 1 {
				// A last line without a newline is not empty.
				return nil, nil, parseError(n, `"\" line follows an empty line`)
			}
			last.Line = last.Line[:len(last.Line)-1]
			endX = endX || last.Op != diff.Insert
			endY = endY || last.Op != diff.Delete
			last = nil
			continue
		}

		switch {
		case state == signature:
			// A signature ends the patch, unless another patch follows, as in
			// the output of git format-patch --stdout.
			if startsPatch(l) {
				return nil, nil, parseError(n, "patch contains more than one file")
			}
		case state == body:
			e := Edit[T]{Op: diff.Match, LineNoX: x, LineNoY: y, Line: patch[start+1 : end]}
			switch l[0] {
			case ' ':
				x++
				y++
			case '\n':
				// An empty line is an empty context line, as git apply accepts.
				e.Line = patch[start:end]
				x++
				y++
			case '-':
				e.Op, e.LineNoY = diff.Delete, -1
				x++
			case '+':
				e.Op, e.LineNoX = diff.Insert, -1
				y++
			default:
				if l.HasPrefix("@@ ") {
					return nil, nil, parseError(n, "hunk is shorter than its header")
				}
				return nil, nil, parseError(n, fmt.Sprintf("unknown line prefix %q", l[0]))
			}
			if x > hk.EndLineNoX || y > hk.EndLineNoY {
				return nil, nil, parseError(n, "hunk is longer than its header")
			}
			if (endX && e.Op != diff.Insert) || (endY && e.Op != diff.Delete) {
				return nil, nil, parseError(n, "line after end of file")
			}
			edits = append(edits, e)
			last = &edits[len(edits)-1]
		case l.HasPrefix("@@ "):
			var ok bool
			if hk, ok = parseHunkHeader[T](l); !ok {
				return nil, nil, parseError(n, "malformed hunk header")
			}
			if hk.LineNoX == hk.EndLineNoX && hk.LineNoY == hk.EndLineNoY {
				return nil, nil, parseError(n, "empty hunk")
			}
			if endX || endY {
				return nil, nil, parseError(n, "hunk after end of file")
			}
			if k := len(hunks); k > 0 && (hk.LineNoX < hunks[k-1].EndLineNoX || hk.LineNoY < hunks[k-1].EndLineNoY) {
				return nil, nil, parseError(n, "hunk overlaps the previous hunk")
			}
			// A hunk has one edit per line. The header counts and the lines
			// left in the patch bound the number of lines, so a malformed
			// patch does not cause a large allocation.
			rest := lineCount - n
			need := min(min(hk.EndLineNoX-hk.LineNoX, rest)+min(hk.EndLineNoY-hk.LineNoY, rest), rest)
			if cap(edits) < need {
				// Room for as many edits as lines read so far grows the
				// buffer geometrically, so a patch with many hunks needs few
				// allocations.
				edits = slices.Grow(edits, max(need, min(n, rest)))
			}
			x, y = hk.LineNoX, hk.LineNoY
			last = nil
			state = body
		case state == fileHeader:
			return nil, nil, parseError(n, "missing hunk header after file header")
		case state == hunkHeader && startsPatch(l):
			return nil, nil, parseError(n, "patch contains more than one file")
		case state == hunkHeader && l == "-- \n":
			state = signature
		case state == hunkHeader:
			return nil, nil, parseError(n, "unexpected text after hunk")
		case l.HasPrefix("--- "):
			labelX = l
			state = headerY
		}

		if state == body && x == hk.EndLineNoX && y == hk.EndLineNoY {
			hk.Edits = slices.Clip(edits)
			edits = edits[len(edits):]
			hunks = append(hunks, hk)
			state = hunkHeader
		}
	}

	switch {
	case state == body:
		return nil, nil, parseError(n+1, "hunk is shorter than its header")
	case state == gitHeader:
		// A git diff of a mode change, a rename or an empty file has no file
		// header and no hunks, so hunks cannot represent it.
		return nil, nil, parseError(n, "git diff without a content change")
	case state != fileHeader && state != hunkHeader && state != signature && len(s) > 0:
		return nil, nil, parseError(n, "no file header or hunk")
	}
	return h, hunks, nil
}

// parseError returns the error for a malformed patch at line n (one-based).
func parseError(n int, msg string) error {
	return fmt.Errorf("line %d: %s", n, msg)
}

// startsPatch reports whether l starts another patch after a hunk or a
// signature: the next file of a diff, or the next mail of git format-patch
// --stdout.
func startsPatch(l mem.UnsafeString) bool {
	return l.HasPrefix("From ") || l.HasPrefix("diff ") || l.HasPrefix("--- ")
}

// isGitExtendedHeader reports whether l is a line that git diff writes between
// the "diff --git" line and the file header.
func isGitExtendedHeader(l mem.UnsafeString) bool {
	for _, p := range [...]string{
		"old mode ", "new mode ", "deleted file mode ", "new file mode ",
		"copy from ", "copy to ", "rename old ", "rename new ", "rename from ",
		"rename to ", "similarity index ", "dissimilarity index ", "index ",
	} {
		if l.HasPrefix(p) {
			return true
		}
	}
	return false
}

// parseHunkHeader returns the hunk with the ranges of the hunk header l, and
// reports whether l is a valid hunk header. l has the format
// "@@ -a[,b] +c[,d] @@", optionally followed by text.
func parseHunkHeader[T string | []byte](l mem.UnsafeString) (Hunk[T], bool) {
	var h Hunk[T]
	if !l.HasPrefix("@@ -") {
		return h, false
	}
	var ok bool
	if h.LineNoX, h.EndLineNoX, l, ok = parseRange(l[len("@@ -"):]); !ok {
		return h, false
	}
	if !l.HasPrefix(" +") {
		return h, false
	}
	if h.LineNoY, h.EndLineNoY, l, ok = parseRange(l[len(" +"):]); !ok {
		return h, false
	}
	return h, l.HasPrefix(" @@")
}

// parseRange parses the range "a[,b]" at the start of s. It returns the range
// as zero-based line numbers [lo, hi), the rest of s, and reports whether s
// starts with a valid range.
func parseRange(s mem.UnsafeString) (lo, hi int, rest mem.UnsafeString, ok bool) {
	start, s, ok := parseNumber(s)
	if !ok {
		return 0, 0, "", false
	}
	count := 1
	if s.HasPrefix(",") {
		if count, s, ok = parseNumber(s[len(","):]); !ok {
			return 0, 0, "", false
		}
	}
	// Line numbers are one-based, except that an empty range names the line
	// before it.
	lo = start
	if count > 0 {
		if start == 0 {
			return 0, 0, "", false
		}
		lo--
	}
	if count > math.MaxInt-lo {
		return 0, 0, "", false
	}
	return lo, lo + count, s, true
}

// parseNumber parses the decimal number at the start of s. It returns the
// number, the rest of s, and reports whether s starts with a number that fits
// in an int.
func parseNumber(s mem.UnsafeString) (int, mem.UnsafeString, bool) {
	i := 0
	for i < len(s) && '0' <= s[i] && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, "", false
	}
	// strconv.Atoi does not keep its argument.
	n, err := strconv.Atoi(string(s[:i]))
	return n, s[i:], err == nil
}

// parseLabel returns the label written as l in a file header. A label in
// double quotes is unquoted. Text after the label and a tab is dropped.
func parseLabel(l mem.UnsafeString) string {
	if l.HasPrefix(`"`) {
		if label, rest, ok := unquoteLabel(l); ok && (rest == "" || rest[0] == '\t') {
			return label
		}
		// BSD diff writes labels unquoted, so a label that starts with a
		// quote but is not quoted is kept as written, as git apply does.
	}
	if i := l.IndexByte('\t'); i >= 0 {
		l = l[:i]
	}
	return l.Clone()
}

// unquoteLabel returns the label that [appendQuoted] writes as the start of s,
// and the rest of s after the closing quote. It reports whether s starts with
// a valid quoted label.
//
// Unlike [strconv.Unquote], it rejects the \x and \u escapes, as git does, and
// keeps bytes that are not UTF-8.
func unquoteLabel(s mem.UnsafeString) (label string, rest mem.UnsafeString, ok bool) {
	var b strings.Builder
	b.Grow(len(s))
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			return b.String(), s[i+1:], true
		case '\\':
			i++
			if i == len(s) {
				return "", "", false
			}
			c = s[i]
			switch j := strings.IndexByte(escapes, c); {
			case c == '"' || c == '\\':
			case j >= 0:
				c = '\a' + byte(j)
			case '0' <= c && c <= '3' && i+2 < len(s) && isOctal(s[i+1]) && isOctal(s[i+2]):
				c = (c-'0')<<6 | (s[i+1]-'0')<<3 | (s[i+2] - '0')
				i += 2
			default:
				return "", "", false
			}
		}
		b.WriteByte(c)
	}
	return "", "", false
}

// isOctal reports whether c is an octal digit.
func isOctal(c byte) bool {
	return '0' <= c && c <= '7'
}
