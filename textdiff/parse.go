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

// File is the diff of one file in a unified diff.
type File[T string | []byte] struct {
	X, Y     string    // Labels after "--- " and "+++ ", unquoted; empty without a file header.
	Extended []string  // Lines of the git diff header before the file header, without newlines.
	Hunks    []Hunk[T] // Hunks of the file.
}

// ParseOption configures [ParseUnified].
type ParseOption func(*parseConfig)

// parseConfig is the configuration of ParseUnified.
type parseConfig struct {
	mode parseMode
}

// parseMode selects the input that ParseUnified accepts.
type parseMode int

const (
	parseStrict parseMode = iota
	parseLenient
	parseHeaderless
)

// ParseLenient skips text between files and after the last file, and accepts a
// hunk that overlaps the previous hunk in y, as patch and git apply do. Line
// numbers in y are then those of the hunk headers.
func ParseLenient() ParseOption {
	return func(c *parseConfig) { c.mode = parseLenient }
}

// ParseHeaderless reads one file without a header: hunks and nothing else.
func ParseHeaderless() ParseOption {
	return func(c *parseConfig) { c.mode = parseHeaderless }
}

// ParseUnified returns the files of a unified diff, as produced by [Unified],
// git diff, GNU diff -u or BSD diff -u. An empty patch returns nil files.
//
// A file starts with a "diff --git" line or a file header. Text before the
// first file is skipped, and so is the signature that git format-patch writes
// after the last file. A label in double quotes is unquoted with C-style
// escapes, and text after a tab in a label, such as a timestamp, is dropped. A
// git diff without a content change, such as a mode change or a rename, is a
// file with Extended and no hunks.
//
// The Line of each edit is a subslice of patch without the line prefix, so it
// changes when a []byte patch does. It ends in a newline, unless a
// "\ No newline at end of file" line follows it. Labels and extended header
// lines are copies.
//
// ParseUnified returns an error that names the line of the patch if the patch
// is malformed, if hunks of a file overlap, if the patch holds a hunk without a
// file header, a binary diff, text after a hunk that does not start a file or a
// signature, or more than one mail of git format-patch. [ParseLenient] and
// [ParseHeaderless] change which input is accepted.
func ParseUnified[T string | []byte](patch T, opts ...ParseOption) ([]File[T], error) {
	var cfg parseConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	s := mem.Of(patch)
	if len(s) > 0 && s[len(s)-1] != '\n' {
		return nil, parseError(s.Count("\n")+1, "missing newline at end of patch")
	}
	p := &parser[T]{patch: patch, s: s, lenient: cfg.mode == parseLenient}
	p.lines = s.Count("\n")
	p.cur = lineAt(s, 0)

	if cfg.mode == parseHeaderless {
		if p.cur == "" {
			return nil, nil
		}
		if !p.cur.HasPrefix("@@ ") {
			return nil, parseError(p.n+1, "missing hunk header")
		}
		var f File[T]
		if err := p.hunks(&f); err != nil {
			return nil, err
		}
		if p.cur != "" {
			return nil, parseError(p.n+1, "unexpected text after hunk")
		}
		return []File[T]{f}, nil
	}

	ok, err := p.seekFile(firstFile)
	if err != nil {
		return nil, err
	}
	if !ok && len(s) > 0 {
		return nil, parseError(p.n, "no file header or hunk")
	}
	var files []File[T]
	for ok {
		f, pos, err := p.file()
		if err != nil {
			return nil, err
		}
		files = append(files, f)
		if ok, err = p.seekFile(pos); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// parser reads a patch line by line.
type parser[T string | []byte] struct {
	patch   T                // The patch; the lines of edits are subslices of it.
	s       mem.UnsafeString // The patch as a string.
	lines   int              // Number of lines in the patch.
	lenient bool             // Reports whether the parser is lenient.

	cur mem.UnsafeString // The next line with its newline; empty at the end of the patch.
	off int              // Offset of cur in the patch.
	n   int              // Number of lines before cur.

	edits      []Edit[T] // Edits of the current hunk, with room for the edits of later hunks.
	endX, endY bool      // Report whether the last line of x or y of the current file has been read.
}

// lineAt returns the line of s that starts at offset off, with its newline,
// or "" if off is the end of s.
func lineAt(s mem.UnsafeString, off int) mem.UnsafeString {
	if off == len(s) {
		return ""
	}
	return s[off : off+s[off:].IndexByte('\n')+1]
}

// advance moves past the line cur and returns it.
func (p *parser[T]) advance() mem.UnsafeString {
	l := p.cur
	p.off += len(l)
	p.n++
	p.cur = lineAt(p.s, p.off)
	return l
}

// atFileHeader reports whether cur and the line after it are a file header.
func (p *parser[T]) atFileHeader() bool {
	return p.cur.HasPrefix("--- ") && lineAt(p.s, p.off+len(p.cur)).HasPrefix("+++ ")
}

// position is where seekFile starts to read, relative to the files before it.
type position int

const (
	firstFile      position = iota // Before the first file.
	afterHunk                      // After a hunk.
	afterGitHeader                 // After a git diff header without a content change.
	afterText                      // After text between files that the lenient parser skipped.
)

// seekFile advances from pos to the next line that starts a file and reports
// whether there is one. Text before the first file is skipped. Text after a
// file is an error unless it is a signature of git format-patch or the parser
// is lenient.
func (p *parser[T]) seekFile(pos position) (bool, error) {
	for p.cur != "" {
		l := p.cur
		switch {
		case l.HasPrefix("diff --git "), p.atFileHeader():
			return true, nil
		case l.HasPrefix("@@ "):
			if _, err := p.hunkHeader(); err != nil {
				return false, err
			}
			return false, parseError(p.n+1, "hunk without file header")
		case pos == firstFile:
			if l.HasPrefix("Binary files ") {
				return false, parseError(p.n+1, "binary diff")
			}
		case pos == afterText:
		case l == "-- \n":
			p.advance()
			if err := p.signature(); err != nil {
				return false, err
			}
			pos = afterText
			continue
		case pos == afterHunk && l.HasPrefix("diff "):
			// diff -r writes a "diff " line before the file header.
			p.advance()
			switch l := p.cur; {
			case l.HasPrefix("diff --git "), p.atFileHeader():
				return true, nil
			case p.lenient && !l.HasPrefix("@@ "):
				pos = afterText
				continue
			case l.HasPrefix("Binary files "):
				return false, parseError(p.n+1, "binary diff")
			case l.HasPrefix("--- "):
				p.advance()
				return false, parseError(p.n+1, `missing "+++ " line after "--- " line`)
			}
			return false, parseError(p.n+1, `missing file header after "diff " line`)
		case p.lenient:
		case l.HasPrefix("--- "):
			p.advance()
			return false, parseError(p.n+1, `missing "+++ " line after "--- " line`)
		case pos == afterHunk:
			return false, parseError(p.n+1, "unexpected text after hunk")
		default:
			return false, parseError(p.n+1, "unexpected text after git diff header")
		}
		p.advance()
		if pos != firstFile {
			pos = afterText
		}
	}
	return false, nil
}

// signature advances past the signature that git format-patch writes after
// its "-- " line. A line that starts another patch ends the signature and is
// an error unless the parser is lenient.
func (p *parser[T]) signature() error {
	for ; p.cur != ""; p.advance() {
		if startsPatch(p.cur) {
			if !p.lenient {
				return parseError(p.n+1, "patch after signature")
			}
			return nil
		}
	}
	return nil
}

// file reads the file that starts at cur: an optional "diff --git" line with
// its extended headers, a file header and hunks. It returns the file and the
// position after it.
func (p *parser[T]) file() (File[T], position, error) {
	var f File[T]
	if p.cur.HasPrefix("diff --git ") {
		for ok := true; ok; ok = isGitExtendedHeader(p.cur) {
			l := p.advance()
			f.Extended = append(f.Extended, l[:len(l)-1].Clone())
		}
		switch {
		case p.cur.HasPrefix("Binary files "), p.cur == "GIT binary patch\n":
			return f, 0, parseError(p.n+1, "binary diff")
		case p.cur.HasPrefix("@@ "):
			// git apply reads hunks after a git diff header without a file
			// header.
			return f, afterHunk, p.hunks(&f)
		case !p.atFileHeader():
			// A git diff without a content change has no file header.
			return f, afterGitHeader, nil
		}
	}
	x, y := p.advance(), p.advance()
	f.X = parseLabel(x[len("--- ") : len(x)-1])
	f.Y = parseLabel(y[len("+++ ") : len(y)-1])
	if p.cur != "" && !p.cur.HasPrefix("@@ ") {
		return f, 0, parseError(p.n+1, "missing hunk header after file header")
	}
	return f, afterHunk, p.hunks(&f)
}

// hunks reads the hunks of f that start at cur.
func (p *parser[T]) hunks(f *File[T]) error {
	p.endX, p.endY = false, false
	for p.cur.HasPrefix("@@ ") {
		hk, err := p.hunk(f.Hunks)
		if err != nil {
			return err
		}
		f.Hunks = append(f.Hunks, hk)
	}
	return nil
}

// hunkHeader returns the hunk of the hunk header at cur, without edits.
func (p *parser[T]) hunkHeader() (Hunk[T], error) {
	hk, ok := parseHunkHeader[T](p.cur)
	switch {
	case !ok:
		return hk, parseError(p.n+1, "malformed hunk header")
	case hk.LineNoX == hk.EndLineNoX && hk.LineNoY == hk.EndLineNoY:
		return hk, parseError(p.n+1, "empty hunk")
	}
	return hk, nil
}

// hunk reads the hunk that starts at cur and follows prev, with the "\" line
// after its last line.
func (p *parser[T]) hunk(prev []Hunk[T]) (Hunk[T], error) {
	hk, err := p.hunkHeader()
	switch {
	case err != nil:
		return hk, err
	case p.endX || p.endY:
		return hk, parseError(p.n+1, "hunk after end of file")
	case len(prev) > 0 && (hk.LineNoX < prev[len(prev)-1].EndLineNoX || !p.lenient && hk.LineNoY < prev[len(prev)-1].EndLineNoY):
		return hk, parseError(p.n+1, "hunk overlaps the previous hunk")
	}
	p.advance()

	// A hunk has one edit per line. The header counts and the lines left in
	// the patch bound the number of lines, so a malformed patch does not
	// cause a large allocation.
	rest := p.lines - p.n
	need := min(min(hk.EndLineNoX-hk.LineNoX, rest)+min(hk.EndLineNoY-hk.LineNoY, rest), rest)
	if cap(p.edits) < need {
		// Room for as many edits as lines read so far grows the buffer
		// geometrically, so a patch with many hunks needs few allocations.
		p.edits = slices.Grow(p.edits, max(need, min(p.n, rest)))
	}

	// The body is read with local copies of the parser state, which the
	// compiler keeps in registers.
	x, y := hk.LineNoX, hk.LineNoY
	edits, off, n := p.edits, p.off, p.n
	endX, endY := p.endX, p.endY
	var last *Edit[T] // The edit of the previous line, if it is a body line.
	for {
		l := lineAt(p.s, off)
		if x == hk.EndLineNoX && y == hk.EndLineNoY && !l.HasPrefix("\\") {
			break
		}
		if l == "" {
			return hk, parseError(n+1, "hunk is shorter than its header")
		}
		if l[0] == '\\' {
			// A "\" line removes the newline of the previous body line.
			switch {
			case last == nil:
				return hk, parseError(n+1, `"\" line does not follow a body line`)
			case len(last.Line) == 1:
				// A last line without a newline is not empty.
				return hk, parseError(n+1, `"\" line follows an empty line`)
			}
			last.Line = last.Line[:len(last.Line)-1]
			endX = endX || last.Op != diff.Insert
			endY = endY || last.Op != diff.Delete
			last = nil
			off += len(l)
			n++
			continue
		}
		e := Edit[T]{Op: diff.Match, LineNoX: x, LineNoY: y, Line: p.patch[off+1 : off+len(l)]}
		switch l[0] {
		case ' ':
			x++
			y++
		case '\n':
			// An empty line is an empty context line, as git apply accepts.
			e.Line = p.patch[off : off+1]
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
				return hk, parseError(n+1, "hunk is shorter than its header")
			}
			return hk, parseError(n+1, fmt.Sprintf("unknown line prefix %q", l[0]))
		}
		if x > hk.EndLineNoX || y > hk.EndLineNoY {
			return hk, parseError(n+1, "hunk is longer than its header")
		}
		if (endX && e.Op != diff.Insert) || (endY && e.Op != diff.Delete) {
			return hk, parseError(n+1, "line after end of file")
		}
		off += len(l)
		n++
		edits = append(edits, e)
		last = &edits[len(edits)-1]
	}
	p.off, p.n, p.endX, p.endY = off, n, endX, endY
	p.cur = lineAt(p.s, off)
	hk.Edits = slices.Clip(edits)
	p.edits = edits[len(edits):]
	return hk, nil
}

// parseError returns the error for a malformed patch at line n (one-based).
func parseError(n int, msg string) error {
	return fmt.Errorf("line %d: %s", n, msg)
}

// startsPatch reports whether l starts another patch after a signature: the
// next mail of git format-patch --stdout, or a diff.
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
