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
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/tools/txtar"
	"znkr.io/diff"
	"znkr.io/diff/internal/config"
	"znkr.io/diff/internal/unixtools"
	"znkr.io/diff/textdiff/color"
)

var (
	update   = flag.Bool("update", false, "update golden files")
	validate = flag.Bool("validate", false, "perform validation using the unix patch cli tool")
)

func TestUnified(t *testing.T) {
	for _, tt := range parseTests(t) {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for sti, st := range tt.subtests {
				t.Run(st.name, func(t *testing.T) {
					t.Parallel()
					got := Unified(tt.x, tt.y, st.opts...)
					// cmp.Diff takes seconds on the large testdata files, so it
					// only runs to report a difference.
					if !bytes.Equal(st.want, got) {
						t.Errorf("Unified(...) result are different:\ngot:\n%s\nwant:\n%s\ndiff [-got,+want]:\n%s", got, st.want, cmp.Diff(st.want, got))
					}
					if *validate && len(got) > 0 {
						patched, err := unixtools.Patch(string(tt.x), string(got))
						if err != nil {
							t.Fatalf("failed to run patch: %v", err)
						}
						if !bytes.Equal(tt.y, []byte(patched)) {
							t.Errorf("file is different after applying patch [-got,+want]:\n%s", cmp.Diff(tt.y, []byte(patched)))
						}
					}
					if *update {
						tt.subtests[sti].want = got
					}
				})
			}

			// Run in a cleanup to makes sure to runs after the subtests have
			// finished.
			t.Cleanup(func() {
				if *update {
					f, err := os.CreateTemp("", "test-unified-*")
					if err != nil {
						t.Fatalf("failed to create temporary file: %v", err)
					}
					defer f.Close()

					write := func(b []byte) {
						t.Helper()
						_, err := f.Write(b)
						if err != nil {
							t.Fatalf("error writing golden file: %v", err)
						}
					}

					write(tt.comment)
					write([]byte("-- x --\n"))
					write(tt.x)
					write([]byte("-- y --\n"))
					write(tt.y)
					for _, st := range tt.subtests {
						write([]byte("-- diff --\n"))
						write(st.pragmas)
						write(st.want)
					}

					if err := f.Close(); err != nil {
						t.Fatalf("error closing golden file: %v", err)
					}
					if err := os.Rename(f.Name(), tt.filename); err != nil {
						t.Fatalf("error renaming golden file: %v", err)
					}
				}
			})
		})
	}
}

func TestAppendUnified(t *testing.T) {
	prefix := []byte("prefix\n")
	for _, tt := range parseTests(t) {
		for _, st := range tt.subtests {
			dst := slices.Clone(prefix)
			got := AppendUnified(dst, tt.x, tt.y, st.opts...)
			want := append(slices.Clone(prefix), st.want...)
			if !bytes.Equal(got, want) {
				t.Errorf("%s/%s: AppendUnified(...) differs from prefix + Unified(...)", tt.name, st.name)
			}
		}
	}
}

func TestWriteUnified(t *testing.T) {
	colors := TerminalColors(color.FileHeaders(4), color.HunkHeaders(33), color.Matches(1), color.Deletes(34), color.Inserts(35))
	for _, tt := range parseTests(t) {
		for _, st := range tt.subtests {
			var buf bytes.Buffer
			// No write fails; failingWriter only rejects empty writes.
			w := io.MultiWriter(&buf, &failingWriter{failAt: -1})
			if err := WriteUnified(w, tt.x, tt.y, st.opts...); err != nil {
				t.Fatalf("%s/%s: WriteUnified(...) = %v", tt.name, st.name, err)
			}
			if !bytes.Equal(buf.Bytes(), st.want) {
				t.Errorf("%s/%s: WriteUnified(...) differs from Unified(...)", tt.name, st.name)
			}

			// With colors and a header, every piece of the output is non-empty.
			opts := append(slices.Clone(st.opts), colors, UnifiedHeader("x", "y"))
			buf.Reset()
			if err := WriteUnified(w, tt.x, tt.y, opts...); err != nil {
				t.Fatalf("%s/%s: WriteUnified(..., TerminalColors(...), UnifiedHeader(...)) = %v", tt.name, st.name, err)
			}
			if want := Unified(tt.x, tt.y, opts...); !bytes.Equal(buf.Bytes(), want) {
				t.Errorf("%s/%s: WriteUnified(..., TerminalColors(...), UnifiedHeader(...)) differs from Unified", tt.name, st.name)
			}
		}
	}
}

type errWriter struct{ err error }

func (w errWriter) Write([]byte) (int, error) { return 0, w.err }

// failingWriter fails on write number failAt (zero-based) and counts the writes
// it receives.
type failingWriter struct {
	n, failAt int
	err       error
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.n++
	if len(p) == 0 {
		return 0, errors.New("empty write")
	}
	if w.n-1 == w.failAt {
		return 0, w.err
	}
	return len(p), nil
}

func TestWriteUnifiedStopsAtError(t *testing.T) {
	// Every line changes, so the output is about 30 * flushSize bytes and
	// WriteUnified writes it in several chunks.
	var x, y strings.Builder
	for i := range 20 * flushSize / 16 {
		fmt.Fprintf(&x, "line %d\n", i)
		fmt.Fprintf(&y, "LINE %d\n", i)
	}
	wantErr := errors.New("write failed")
	for failAt := range 5 {
		w := &failingWriter{failAt: failAt, err: wantErr}
		if err := WriteUnified(w, x.String(), y.String()); err != wantErr {
			t.Fatalf("failAt=%d: WriteUnified(...) = %v, want %v", failAt, err, wantErr)
		}
		if w.n != failAt+1 {
			t.Errorf("failAt=%d: WriteUnified(...) made %d writes, want %d", failAt, w.n, failAt+1)
		}
	}
}

// chunkWriter records the size of every write.
type chunkWriter struct {
	buf    bytes.Buffer
	chunks []int
}

func (w *chunkWriter) Write(p []byte) (int, error) {
	w.chunks = append(w.chunks, len(p))
	return w.buf.Write(p)
}

func TestWriteUnifiedChunks(t *testing.T) {
	var x, y strings.Builder
	for i := range 20 * flushSize / 16 {
		fmt.Fprintf(&x, "line %d\n", i)
		fmt.Fprintf(&y, "LINE %d\n", i)
	}
	var w chunkWriter
	if err := WriteUnified(&w, x.String(), y.String()); err != nil {
		t.Fatalf("WriteUnified(...) = %v", err)
	}
	if got, want := w.buf.String(), Unified(x.String(), y.String()); got != want {
		t.Fatalf("WriteUnified(...) differs from Unified(...)")
	}
	if len(w.chunks) < 2 {
		t.Fatalf("WriteUnified(...) made %d writes, want several", len(w.chunks))
	}
	for i, n := range w.chunks {
		// A chunk holds at most flushSize bytes plus the line that crossed it.
		if n == 0 || n > flushSize+64 {
			t.Errorf("write %d has %d bytes, want 1 to %d", i, n, flushSize+64)
		}
	}
}

func TestWriteUnifiedError(t *testing.T) {
	wantErr := errors.New("write failed")
	if err := WriteUnified(errWriter{wantErr}, "a\n", "b\n"); err != wantErr {
		t.Errorf("WriteUnified(...) = %v, want %v", err, wantErr)
	}
	if err := WriteUnified(errWriter{wantErr}, "a\n", "a\n"); err != nil {
		t.Errorf("WriteUnified(...) for identical inputs = %v, want nil", err)
	}
}

func TestUnifiedEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		x, y string
		want string
	}{
		{
			name: "empty",
			x:    "",
			y:    "",
			want: "",
		},
		{
			name: "identical",
			x:    "first line\n",
			y:    "first line\n",
			want: "",
		},
		{
			name: "new-lines-only",
			x:    "\n",
			y:    "\n",
			want: "",
		},
		{
			name: "x-empty",
			x:    "",
			y:    "one-line\n",
			want: "@@ -0,0 +1 @@\n+one-line\n",
		},
		{
			name: "y-empty",
			x:    "one-line\n",
			y:    "",
			want: "@@ -1 +0,0 @@\n-one-line\n",
		},
		{
			name: "missing-newline-x",
			x:    "first line",
			y:    "first line\n",
			want: "@@ -1 +1 @@\n-first line\n\\ No newline at end of file\n+first line\n",
		},
		{
			name: "missing-newline-y",
			x:    "first line\n",
			y:    "first line",
			want: "@@ -1 +1 @@\n-first line\n+first line\n\\ No newline at end of file\n",
		},
		{
			name: "missing-newline-both",
			x:    "a\nsecond line",
			y:    "b\nsecond line",
			want: "@@ -1,2 +1,2 @@\n-a\n+b\n second line\n\\ No newline at end of file\n",
		},
		{
			name: "missing-newline-outside-hunk",
			x:    "a\n1\n2\n3\n4\nlast line",
			y:    "b\n1\n2\n3\n4\nlast line",
			want: "@@ -1,4 +1,4 @@\n-a\n+b\n 1\n 2\n 3\n",
		},
		{
			name: "missing-newline-empty-x",
			x:    "",
			y:    "\n",
			want: "@@ -0,0 +1 @@\n+\n", // no missing newline note here
		},
		{
			name: "missing-newline-empty-y",
			x:    "\n",
			y:    "",
			want: "@@ -1 +0,0 @@\n-\n", // no missing newline note here
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unified(tt.x, tt.y)
			if got != tt.want {
				t.Errorf("Unified(...) if different:\ngot:  %q\nwant: %q", got, tt.want)
			}
			if *validate && len(got) > 0 {
				patched, err := unixtools.Patch(tt.x, got)
				if err != nil {
					t.Fatalf("failed to run patch: %v", err)
				}
				if diff := cmp.Diff(tt.y, patched); diff != "" {
					t.Errorf("file is different after applying patch [-got,+want]:\n%s", diff)
				}
			}
		})
	}
}

func TestUnifiedColors(t *testing.T) {
	tests := []struct {
		name string
		x, y string
		opts []diff.Option
		want string
	}{
		{
			name: "no-colors",
			x: `this paragraph
stays but is
not long enough
to create a
new hunk

this paragraph
is going to be
removed
`,
			y: `this is a new paragraph
that is inserted at the top

this paragraph
stays but is
not long enough
to create a
new hunk
`,
			want: `@@ -1,9 +1,8 @@
+this is a new paragraph
+that is inserted at the top
+
 this paragraph
 stays but is
 not long enough
 to create a
 new hunk
-
-this paragraph
-is going to be
-removed
`,
		},
		{
			name: "default-colors",
			x: `this paragraph
stays but is
not long enough
to create a
new hunk

this paragraph
is going to be
removed
`,
			y: `this is a new paragraph
that is inserted at the top

this paragraph
stays but is
not long enough
to create a
new hunk
`,
			opts: []diff.Option{TerminalColors()},
			want: "\x1b[36m" + `@@ -1,9 +1,8 @@` + "\x1b[m" + `
` + "\x1b[32m" + `+this is a new paragraph
+that is inserted at the top
+
` + "\x1b[m" + ` this paragraph
 stays but is
 not long enough
 to create a
 new hunk
` + "\x1b[m\x1b[31m" + `-
-this paragraph
-is going to be
-removed
` + "\x1b[m",
		},
		{
			name: "custom-colors",
			x: `this paragraph
stays but is
not long enough
to create a
new hunk

this paragraph
is going to be
removed
`,
			y: `this is a new paragraph
that is inserted at the top

this paragraph
stays but is
not long enough
to create a
new hunk
`,
			opts: []diff.Option{TerminalColors(
				color.HunkHeaders(33), // Yellow
				color.Matches(1, 33),  // Bold Yellow
				color.Deletes(34),     // Blue
				color.Inserts(35),     // Magenta
			)},
			want: "\x1b[33m" + `@@ -1,9 +1,8 @@` + "\x1b[m" + `
` + "\x1b[35m" + `+this is a new paragraph
+that is inserted at the top
+
` + "\x1b[m\x1b[1;33m" + ` this paragraph
 stays but is
 not long enough
 to create a
 new hunk
` + "\x1b[m\x1b[34m" + `-
-this paragraph
-is going to be
-removed
` + "\x1b[m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unified(tt.x, tt.y, tt.opts...)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Unified(...) result are different:\ngot:\n%s\nwant:\n%s\ndiff [-got,+want]:\n%s", got, tt.want, diff)
			}
		})
	}
}

func TestUnifiedHeader(t *testing.T) {
	tests := []struct {
		name string
		x, y string
		opts []diff.Option
		want string
	}{
		{
			name: "labels",
			x:    "a\nb\n",
			y:    "a\nc\n",
			opts: []diff.Option{UnifiedHeader("x.txt", "y.txt")},
			want: `--- x.txt
+++ y.txt
@@ -1,2 +1,2 @@
 a
-b
+c
`,
		},
		{
			name: "identical",
			x:    "a\n",
			y:    "a\n",
			opts: []diff.Option{UnifiedHeader("x.txt", "y.txt")},
			want: "",
		},
		{
			name: "empty",
			x:    "",
			y:    "",
			opts: []diff.Option{UnifiedHeader("x.txt", "y.txt")},
			want: "",
		},
		{
			name: "empty-labels",
			x:    "a\n",
			y:    "b\n",
			opts: []diff.Option{UnifiedHeader("", "")},
			want: "--- \n+++ \n" + `@@ -1 +1 @@
-a
+b
`,
		},
		{
			name: "tab",
			x:    "a\n",
			y:    "b\n",
			opts: []diff.Option{UnifiedHeader("x\t.txt", "y.txt\t")},
			want: `--- "x\t.txt"
+++ "y.txt\t"
@@ -1 +1 @@
-a
+b
`,
		},
		{
			name: "quoted",
			x:    "a\n",
			y:    "b\n",
			opts: []diff.Option{UnifiedHeader("a/x\ny\t\"q\"\\", "b/\x00\x1b\x7f\r\a\b\v\fé")},
			want: `--- "a/x\ny\t\"q\"\\"
+++ "b/\000\033\177\r\a\b\v\fé"
@@ -1 +1 @@
-a
+b
`,
		},
		{
			name: "leading-quote",
			x:    "a\n",
			y:    "b\n",
			opts: []diff.Option{UnifiedHeader(`"a/x\ny"`, `"b".txt`)},
			want: `--- "\"a/x\\ny\""
+++ "\"b\".txt"
@@ -1 +1 @@
-a
+b
`,
		},
		{
			name: "space",
			x:    "a\n",
			y:    "b\n",
			opts: []diff.Option{UnifiedHeader("a/x y.txt", "b/x y.txt")},
			want: `--- a/x y.txt` + "\t" + `
+++ b/x y.txt` + "\t" + `
@@ -1 +1 @@
-a
+b
`,
		},
		{
			name: "space-tab",
			x:    "a\n",
			y:    "b\n",
			opts: []diff.Option{UnifiedHeader("x y\t.txt", "y y.txt\t")},
			want: `--- "x y\t.txt"` + "\t" + `
+++ "y y.txt\t"` + "\t" + `
@@ -1 +1 @@
-a
+b
`,
		},
		{
			name: "space-quoted",
			x:    "a\n",
			y:    "b\n",
			opts: []diff.Option{UnifiedHeader("a/x y\n", "b/x y\tz\n")},
			want: `--- "a/x y\n"` + "\t" + `
+++ "b/x y\tz\n"` + "\t" + `
@@ -1 +1 @@
-a
+b
`,
		},
		{
			name: "quote-and-backslash",
			x:    "a\n",
			y:    "b\n",
			opts: []diff.Option{UnifiedHeader(`a/"x"`, `b/x\y`)},
			want: `--- "a/\"x\""
+++ "b/x\\y"
@@ -1 +1 @@
-a
+b
`,
		},
		{
			name: "not-quoted",
			x:    "a\n",
			y:    "b\n",
			opts: []diff.Option{UnifiedHeader("a/é-ü.txt", "b/x'y$z.txt")},
			want: `--- a/é-ü.txt
+++ b/x'y$z.txt
@@ -1 +1 @@
-a
+b
`,
		},
		{
			name: "last-wins",
			x:    "a\n",
			y:    "b\n",
			opts: []diff.Option{UnifiedHeader("old", "old"), UnifiedHeader("x.txt", "y.txt")},
			want: `--- x.txt
+++ y.txt
@@ -1 +1 @@
-a
+b
`,
		},
		{
			name: "colors",
			x:    "a\n",
			y:    "b\n",
			opts: []diff.Option{UnifiedHeader("x.txt", "y.txt"), TerminalColors()},
			want: "\x1b[1m" + `--- x.txt` + "\x1b[m" + `
` + "\x1b[1m" + `+++ y.txt` + "\x1b[m" + `
` + "\x1b[36m" + `@@ -1 +1 @@` + "\x1b[m" + `
` + "\x1b[31m" + `-a
` + "\x1b[m\x1b[32m" + `+b
` + "\x1b[m",
		},
		{
			name: "colors-custom",
			x:    "a\n",
			y:    "b\n",
			opts: []diff.Option{UnifiedHeader("x.txt", "y.txt"), TerminalColors(color.FileHeaders(33))},
			want: "\x1b[33m" + `--- x.txt` + "\x1b[m" + `
` + "\x1b[33m" + `+++ y.txt` + "\x1b[m" + `
` + "\x1b[36m" + `@@ -1 +1 @@` + "\x1b[m" + `
` + "\x1b[31m" + `-a
` + "\x1b[m\x1b[32m" + `+b
` + "\x1b[m",
		},
		{
			name: "colors-space",
			x:    "a\n",
			y:    "b\n",
			opts: []diff.Option{UnifiedHeader("x y.txt", "y y.txt"), TerminalColors()},
			want: "\x1b[1m" + `--- x y.txt` + "\x1b[m\t" + `
` + "\x1b[1m" + `+++ y y.txt` + "\x1b[m\t" + `
` + "\x1b[36m" + `@@ -1 +1 @@` + "\x1b[m" + `
` + "\x1b[31m" + `-a
` + "\x1b[m\x1b[32m" + `+b
` + "\x1b[m",
		},
		{
			name: "colors-before-labels",
			x:    "a\n",
			y:    "b\n",
			opts: []diff.Option{TerminalColors(), UnifiedHeader("x.txt", "y.txt")},
			want: "\x1b[1m" + `--- x.txt` + "\x1b[m" + `
` + "\x1b[1m" + `+++ y.txt` + "\x1b[m" + `
` + "\x1b[36m" + `@@ -1 +1 @@` + "\x1b[m" + `
` + "\x1b[31m" + `-a
` + "\x1b[m\x1b[32m" + `+b
` + "\x1b[m",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unified(tt.x, tt.y, tt.opts...)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Unified(...) result are different:\ngot:  %q\nwant: %q\ndiff [-got,+want]:\n%s", got, tt.want, diff)
			}

			prefix := []byte("prefix\n")
			gotAppend := AppendUnified(slices.Clone(prefix), tt.x, tt.y, tt.opts...)
			if want := string(prefix) + tt.want; string(gotAppend) != want {
				t.Errorf("AppendUnified(...) = %q, want %q", gotAppend, want)
			}

			var w chunkWriter
			if err := WriteUnified(&w, tt.x, tt.y, tt.opts...); err != nil {
				t.Fatalf("WriteUnified(...) = %v", err)
			}
			if w.buf.String() != tt.want {
				t.Errorf("WriteUnified(...) wrote %q, want %q", w.buf.String(), tt.want)
			}
			if tt.want == "" && len(w.chunks) != 0 {
				t.Errorf("WriteUnified(...) made %d writes for an empty diff, want 0", len(w.chunks))
			}
		})
	}
}

// TestScanLabelSize checks that the size from scanLabel equals the number of
// bytes appendFileHeader writes for the label.
func TestScanLabelSize(t *testing.T) {
	labels := []string{"", "a/x y.txt", "\"a/x\ny\"", "x y\tz"}
	// Every byte, alone and after a space, which adds a tab.
	for c := range 256 {
		labels = append(labels, string([]byte{byte(c)}), " "+string([]byte{byte(c)}))
	}
	for _, label := range labels {
		l := scanLabel(label)
		got := len(appendFileHeader(nil, "", l, config.ColorConfig{})) - len("\n")
		if got != l.size {
			t.Errorf("scanLabel(%q).size = %d, appendFileHeader wrote %d bytes", label, l.size, got)
		}
	}
}

// TestUnifiedHeaderApply checks that git apply and patch find the file to
// patch from the header alone.
func TestUnifiedHeaderApply(t *testing.T) {
	if !*validate {
		t.Skip("requires -validate")
	}
	const x, y = "one\ntwo\nthree\n", "one\n2\nthree\n"
	tests := []struct {
		name   string
		quoted bool // Reports whether the header writes name in double quotes.
	}{
		{name: "plain.txt"},
		{name: "dir/plain.txt"},
		{name: "with space.txt"},
		{name: `quo"te.txt`, quoted: true},
		{name: `back\slash.txt`, quoted: true},
		{name: "nön-ascii.txt"},
		{name: "new\nline.txt", quoted: true},
		{name: "esc\x1b.txt", quoted: true},
		{name: `"leading-quote".txt`, quoted: true},
		{name: "space and\nnewline.txt", quoted: true},
		{name: "tab\t.txt", quoted: true},
	}
	tools := []struct {
		name  string
		apply func(name, orig, diff string) (string, error)
		// BSD patch, the default on macOS, reads a quoted name literally,
		// quotes and escapes included. git apply and GNU patch decode it.
		decodesQuotes bool
	}{
		{"git apply", unixtools.GitApply, true},
		{"patch", unixtools.PatchFile, unixtools.IsGNUPatch()},
	}
	for _, tool := range tools {
		for _, tt := range tests {
			if tt.quoted && !tool.decodesQuotes {
				continue
			}
			// GNU patch 2.8 decodes a name with a newline, then fails with
			// EILSEQ to rename its temporary file to that name.
			if tool.name == "patch" && strings.Contains(tt.name, "\n") {
				continue
			}
			t.Run(tool.name+"/"+strconv.Quote(tt.name), func(t *testing.T) {
				d := Unified(x, y, UnifiedHeader("a/"+tt.name, "b/"+tt.name))
				got, err := tool.apply(tt.name, x, d)
				if err != nil {
					t.Fatalf("%s failed: %v\ndiff:\n%s", tool.name, err, d)
				}
				if got != y {
					t.Errorf("%s produced %q, want %q\ndiff:\n%s", tool.name, got, y, d)
				}
			})
		}
	}
}

func BenchmarkUnified(b *testing.B) {
	for _, tt := range parseTests(b) {
		b.Run(tt.name, func(b *testing.B) {
			for _, st := range tt.subtests {
				b.Run(st.name, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						_ = Unified(tt.x, tt.y, st.opts...)
					}
				})
			}
		})
	}
}

// BenchmarkUnifiedSmallFiles diffs every small test input once per iteration,
// which is the workload of a tool that diffs many small files.
func BenchmarkUnifiedSmallFiles(b *testing.B) {
	var tests []test
	for _, tt := range parseTests(b) {
		if len(tt.x)+len(tt.y) < 64<<10 {
			tests = append(tests, tt)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		for _, tt := range tests {
			_ = Unified(tt.x, tt.y, IndentHeuristic())
		}
	}
}

// BenchmarkAppendUnifiedSmallFiles is BenchmarkUnifiedSmallFiles, but it
// appends to a reused buffer.
func BenchmarkAppendUnifiedSmallFiles(b *testing.B) {
	var tests []test
	for _, tt := range parseTests(b) {
		if len(tt.x)+len(tt.y) < 64<<10 {
			tests = append(tests, tt)
		}
	}
	b.ReportAllocs()
	var buf []byte
	for b.Loop() {
		for _, tt := range tests {
			buf = AppendUnified(buf[:0], tt.x, tt.y, IndentHeuristic())
		}
	}
}

// BenchmarkWriteUnifiedSmallFiles is BenchmarkUnifiedSmallFiles, but it writes
// to io.Discard.
func BenchmarkWriteUnifiedSmallFiles(b *testing.B) {
	var tests []test
	for _, tt := range parseTests(b) {
		if len(tt.x)+len(tt.y) < 64<<10 {
			tests = append(tests, tt)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		for _, tt := range tests {
			if err := WriteUnified(io.Discard, tt.x, tt.y, IndentHeuristic()); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func TestHunks(t *testing.T) {
	tests := []struct {
		name string
		x, y string
		opts []diff.Option
		want []Hunk[string]
	}{
		{
			name: "identical",
			x:    "foo\nbar\nbaz\n",
			y:    "foo\nbar\nbaz\n",
			want: nil,
		},
		{
			name: "empty",
			want: nil,
		},
		{
			name: "x-empty",
			y:    "foo\nbar\nbaz\n",
			want: []Hunk[string]{
				{
					LineNoX:    0,
					LineNoY:    0,
					EndLineNoX: 0,
					EndLineNoY: 3,
					Edits: []Edit[string]{
						{diff.Insert, -1, 0, "foo\n"},
						{diff.Insert, -1, 1, "bar\n"},
						{diff.Insert, -1, 2, "baz\n"},
					},
				},
			},
		},
		{
			name: "y-empty",
			x:    "foo\nbar\nbaz\n",
			want: []Hunk[string]{
				{
					LineNoX:    0,
					LineNoY:    0,
					EndLineNoX: 3,
					EndLineNoY: 0,
					Edits: []Edit[string]{
						{diff.Delete, 0, -1, "foo\n"},
						{diff.Delete, 1, -1, "bar\n"},
						{diff.Delete, 2, -1, "baz\n"},
					},
				},
			},
		},
		{
			name: "same-prefix",
			x:    "foo\nbar\n",
			y:    "foo\nbaz\n",
			want: []Hunk[string]{
				{
					LineNoX:    0,
					EndLineNoX: 2,
					LineNoY:    0,
					EndLineNoY: 2,
					Edits: []Edit[string]{
						{diff.Match, 0, 0, "foo\n"},
						{diff.Delete, 1, -1, "bar\n"},
						{diff.Insert, -1, 1, "baz\n"},
					},
				},
			},
		},
		{
			name: "same-suffix",
			x:    "foo\nbar\n",
			y:    "loo\nbar\n",
			want: []Hunk[string]{
				{
					LineNoX:    0,
					EndLineNoX: 2,
					LineNoY:    0,
					EndLineNoY: 2,
					Edits: []Edit[string]{
						{diff.Delete, 0, -1, "foo\n"},
						{diff.Insert, -1, 0, "loo\n"},
						{diff.Match, 1, 1, "bar\n"},
					},
				},
			},
		},
		{
			name: "ABCABBA_to_CBABAC",
			x:    "A\nB\nC\nA\nB\nB\nA\n",
			y:    "C\nB\nA\nB\nA\nC\n",
			want: []Hunk[string]{
				{
					LineNoX:    0,
					LineNoY:    0,
					EndLineNoX: 7,
					EndLineNoY: 6,
					Edits: []Edit[string]{
						{diff.Delete, 0, -1, "A\n"},
						{diff.Insert, -1, 0, "C\n"},
						{diff.Match, 1, 1, "B\n"},
						{diff.Delete, 2, -1, "C\n"},
						{diff.Match, 3, 2, "A\n"},
						{diff.Match, 4, 3, "B\n"},
						{diff.Delete, 5, -1, "B\n"},
						{diff.Match, 6, 4, "A\n"},
						{diff.Insert, -1, 5, "C\n"},
					},
				},
			},
		},
		{
			name: "ABCABBA_to_CBABAC_no_context",
			x:    "A\nB\nC\nA\nB\nB\nA\n",
			y:    "C\nB\nA\nB\nA\nC\n",
			opts: []diff.Option{diff.Context(0)},
			want: []Hunk[string]{
				{
					LineNoX:    0,
					LineNoY:    0,
					EndLineNoX: 1,
					EndLineNoY: 1,
					Edits: []Edit[string]{
						{diff.Delete, 0, -1, "A\n"},
						{diff.Insert, -1, 0, "C\n"},
					},
				},
				{
					LineNoX:    2,
					LineNoY:    2,
					EndLineNoX: 3,
					EndLineNoY: 2,
					Edits: []Edit[string]{
						{diff.Delete, 2, -1, "C\n"},
					},
				},
				{
					LineNoX:    5,
					LineNoY:    4,
					EndLineNoX: 6,
					EndLineNoY: 4,
					Edits: []Edit[string]{
						{diff.Delete, 5, -1, "B\n"},
					},
				},
				{
					LineNoX:    7,
					LineNoY:    5,
					EndLineNoX: 7,
					EndLineNoY: 6,
					Edits: []Edit[string]{
						{diff.Insert, -1, 5, "C\n"},
					},
				},
			},
		},
		{
			name: "two-hunks",
			x: `this paragraph
is not
changed and
barely long
enough to
create a
new hunk

this paragraph
is going to be
removed
`,
			y: `this is a new paragraph
that is inserted at the top

this paragraph
is not
changed and
barely long
enough to
create a
new hunk
`,
			want: []Hunk[string]{
				{
					LineNoX:    0,
					EndLineNoX: 3,
					LineNoY:    0,
					EndLineNoY: 6,
					Edits: []Edit[string]{
						{diff.Insert, -1, 0, "this is a new paragraph\n"},
						{diff.Insert, -1, 1, "that is inserted at the top\n"},
						{diff.Insert, -1, 2, "\n"},
						{diff.Match, 0, 3, "this paragraph\n"},
						{diff.Match, 1, 4, "is not\n"},
						{diff.Match, 2, 5, "changed and\n"},
					},
				},
				{
					LineNoX:    4,
					EndLineNoX: 11,
					LineNoY:    7,
					EndLineNoY: 10,
					Edits: []Edit[string]{
						{diff.Match, 4, 7, "enough to\n"},
						{diff.Match, 5, 8, "create a\n"},
						{diff.Match, 6, 9, "new hunk\n"},
						{diff.Delete, 7, -1, "\n"},
						{diff.Delete, 8, -1, "this paragraph\n"},
						{diff.Delete, 9, -1, "is going to be\n"},
						{diff.Delete, 10, -1, "removed\n"},
					},
				},
			},
		},
		{
			name: "overlapping-consecutive-hunks-are-merged",
			x: `this paragraph
stays but is
not long enough
to create a
new hunk

this paragraph
is going to be
removed
`,
			y: `this is a new paragraph
that is inserted at the top

this paragraph
stays but is
not long enough
to create a
new hunk
`,
			want: []Hunk[string]{
				{
					LineNoX:    0,
					EndLineNoX: 9,
					LineNoY:    0,
					EndLineNoY: 8,
					Edits: []Edit[string]{
						{diff.Insert, -1, 0, "this is a new paragraph\n"},
						{diff.Insert, -1, 1, "that is inserted at the top\n"},
						{diff.Insert, -1, 2, "\n"},
						{diff.Match, 0, 3, "this paragraph\n"},
						{diff.Match, 1, 4, "stays but is\n"},
						{diff.Match, 2, 5, "not long enough\n"},
						{diff.Match, 3, 6, "to create a\n"},
						{diff.Match, 4, 7, "new hunk\n"},
						{diff.Delete, 5, -1, "\n"},
						{diff.Delete, 6, -1, "this paragraph\n"},
						{diff.Delete, 7, -1, "is going to be\n"},
						{diff.Delete, 8, -1, "removed\n"},
					},
				},
			},
		},
		{
			name: "indent-heuristic",
			x: `["foo", "bar", "baz"].map do |i|
  i.upcase
end
`,
			y: `["foo", "bar", "baz"].map do |i|
  i
end

["foo", "bar", "baz"].map do |i|
  i.upcase
end
`,
			opts: []diff.Option{IndentHeuristic()},
			want: []Hunk[string]{
				{
					LineNoX:    0,
					EndLineNoX: 3,
					LineNoY:    0,
					EndLineNoY: 7,
					Edits: []Edit[string]{
						{diff.Insert, -1, 0, `["foo", "bar", "baz"].map do |i|` + "\n"},
						{diff.Insert, -1, 1, `  i` + "\n"},
						{diff.Insert, -1, 2, `end` + "\n"},
						{diff.Insert, -1, 3, "\n"},
						{diff.Match, 0, 4, `["foo", "bar", "baz"].map do |i|` + "\n"},
						{diff.Match, 1, 5, `  i.upcase` + "\n"},
						{diff.Match, 2, 6, `end` + "\n"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Hunks(tt.x, tt.y, tt.opts...)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("HunksFunc(...) result is different [-want, +got]:\n%s", diff)
			}
		})
	}
}

func TestEdits(t *testing.T) {
	tests := []struct {
		name string
		x, y string
		opts []diff.Option
		want []Edit[string]
	}{
		{
			name: "identical",
			x:    "foo\nbar\nbaz\n",
			y:    "foo\nbar\nbaz\n",
			want: []Edit[string]{
				{diff.Match, 0, 0, "foo\n"},
				{diff.Match, 1, 1, "bar\n"},
				{diff.Match, 2, 2, "baz\n"},
			},
		},
		{
			name: "empty",
		},
		{
			name: "x-empty",
			y:    "foo\nbar\nbaz\n",
			want: []Edit[string]{
				{diff.Insert, -1, 0, "foo\n"},
				{diff.Insert, -1, 1, "bar\n"},
				{diff.Insert, -1, 2, "baz\n"},
			},
		},
		{
			name: "y-empty",
			x:    "foo\nbar\nbaz\n",
			want: []Edit[string]{
				{diff.Delete, 0, -1, "foo\n"},
				{diff.Delete, 1, -1, "bar\n"},
				{diff.Delete, 2, -1, "baz\n"},
			},
		},
		{
			name: "ABCABBA_to_CBABAC",
			x:    "A\nB\nC\nA\nB\nB\nA\n",
			y:    "C\nB\nA\nB\nA\nC\n",
			want: []Edit[string]{
				{diff.Delete, 0, -1, "A\n"},
				{diff.Insert, -1, 0, "C\n"},
				{diff.Match, 1, 1, "B\n"},
				{diff.Delete, 2, -1, "C\n"},
				{diff.Match, 3, 2, "A\n"},
				{diff.Match, 4, 3, "B\n"},
				{diff.Delete, 5, -1, "B\n"},
				{diff.Match, 6, 4, "A\n"},
				{diff.Insert, -1, 5, "C\n"},
			},
		},
		{
			name: "same-prefix",
			x:    "foo\nbar\n",
			y:    "foo\nbaz\n",
			want: []Edit[string]{
				{diff.Match, 0, 0, "foo\n"},
				{diff.Delete, 1, -1, "bar\n"},
				{diff.Insert, -1, 1, "baz\n"},
			},
		},
		{
			name: "same-suffix",
			x:    "foo\nbar\n",
			y:    "loo\nbar\n",
			want: []Edit[string]{
				{diff.Delete, 0, -1, "foo\n"},
				{diff.Insert, -1, 0, "loo\n"},
				{diff.Match, 1, 1, "bar\n"},
			},
		},
		{
			name: "indent-heuristic",
			x: `["foo", "bar", "baz"].map do |i|
  i.upcase
end
`,
			y: `["foo", "bar", "baz"].map do |i|
  i
end

["foo", "bar", "baz"].map do |i|
  i.upcase
end
`,
			opts: []diff.Option{IndentHeuristic()},
			want: []Edit[string]{
				{diff.Insert, -1, 0, `["foo", "bar", "baz"].map do |i|` + "\n"},
				{diff.Insert, -1, 1, `  i` + "\n"},
				{diff.Insert, -1, 2, `end` + "\n"},
				{diff.Insert, -1, 3, "\n"},
				{diff.Match, 0, 4, `["foo", "bar", "baz"].map do |i|` + "\n"},
				{diff.Match, 1, 5, `  i.upcase` + "\n"},
				{diff.Match, 2, 6, `end` + "\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Edits(tt.x, tt.y, tt.opts...)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Edits(...) result is different (-want, +got):\n%s", diff)
			}
		})
	}
}

type test struct {
	name     string
	filename string
	comment  []byte
	x, y     []byte
	subtests []subtest
}

type subtest struct {
	name    string
	opts    []config.Option
	pragmas []byte
	want    []byte
}

func parseTests(t testing.TB) []test {
	t.Helper()
	testFiles, err := filepath.Glob("testdata/*.test")
	if err != nil {
		t.Fatalf("Failed to read testdata: %v", err)
	}
	var tests []test
	for _, filename := range testFiles {
		ar, err := txtar.ParseFile(filename)
		if err != nil {
			t.Fatalf("failed to parse test case: %v", err)
		}
		name := strings.TrimPrefix(filename, "testdata/")
		test := test{
			name:     name,
			filename: filename,
			comment:  ar.Comment,
		}

		for _, f := range ar.Files {
			switch f.Name {
			case "x":
				test.x = f.Data
			case "y":
				test.y = f.Data
			case "diff":
				data := f.Data
				var st subtest
				var name []string
				i := 0
				for ; i < len(data); i++ {
					if data[i] != '#' {
						break
					}
					i++
					eol := i + bytes.IndexByte(data[i:], '\n')
					if eol < i {
						t.Fatal("failed to parse test case: missing newline after pragma line")
					}
					k, v, found := bytes.Cut(data[i:eol], []byte{':'})
					if !found {
						t.Fatal("failed to parse test case: missing ':' in pragma line")
					}
					switch k, v := strings.TrimSpace(string(k)), strings.TrimSpace(string(v)); k {
					case "indent-heuristic":
						switch v {
						case "true":
							st.opts = append(st.opts, IndentHeuristic())
						case "false":
							// do nothing
						default:
							t.Fatalf("invalid value for indent-heuristic: %q", v)
						}
						name = append(name, k)
					case "force-anchoring-heuristic":
						switch v {
						case "true":
							// The inline function definition is necessary,
							// because the anchoring heuristic is not exported
							// as an option.
							st.opts = append(st.opts, func(cfg *config.Config) config.Flag {
								cfg.ForceAnchoringHeuristic = true
								return 0
							})
						case "false":
							// do nothing
						default:
							t.Fatalf("invalid value for force-anchoring-heuristic: %q", v)
						}
						name = append(name, k)
					case "fast":
						switch v {
						case "true":
							st.opts = append(st.opts, diff.Fast())
						case "false":
							// do nothing
						default:
							t.Fatalf("invalid value for fast: %q", v)
						}
					case "context":
						n, err := strconv.ParseInt(v, 10, 64)
						if err != nil {
							t.Fatalf("invalid value for context: %v", err.Error())
						}
						st.opts = append(st.opts, diff.Context(int(n)))
						name = append(name, k+"="+v)
					default:
						t.Fatalf("unknown option: %q", k)
					}
					i = eol
				}
				if len(name) == 0 {
					name = append(name, "default")
				}
				st.name = strings.Join(name, ":")
				st.pragmas = data[:i]
				st.want = data[i:]
				test.subtests = append(test.subtests, st)
			default:
				t.Fatalf("unknown file in archive: %v", f)
			}
		}
		tests = append(tests, test)
	}
	return tests
}
