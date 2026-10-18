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
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/tools/txtar"
	"znkr.io/diff"
	"znkr.io/diff/internal/benchdata"
)

// TestParseUnifiedRoundTrip checks that ParseUnified returns the hunks of the
// diff that Unified writes for the goldens.
func TestParseUnifiedRoundTrip(t *testing.T) {
	for _, tt := range parseTests(t) {
		for _, n := range []int{0, 3, 1 << 20} {
			t.Run(fmt.Sprintf("%s/context=%d", tt.name, n), func(t *testing.T) {
				t.Parallel()
				checkRoundTrip(t, tt.x, tt.y, n)
			})
		}
	}
}

// TestParseUnifiedEdgeCases checks that ParseUnified returns the hunks of the
// diff that Unified writes for inputs at the edges of the format.
func TestParseUnifiedEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		x, y string
	}{
		{name: "empty"},
		{name: "identical", x: "a\n", y: "a\n"},
		{name: "x-empty", y: "a\n"},
		{name: "y-empty", x: "a\n"},
		{name: "missing-newline-x", x: "a", y: "a\n"},
		{name: "missing-newline-y", x: "a\n", y: "a"},
		{name: "missing-newline-both", x: "a\nb", y: "c\nb"},
		{name: "missing-newline-outside-hunk", x: "a\n1\n2\n3\n4\nb", y: "c\n1\n2\n3\n4\nb"},
		{name: "empty-lines", x: "\n\n\n", y: "\n\nx\n\n"},
		{name: "insert-middle", x: "a\nb\nc\nd\ne\nf\ng\nh\n", y: "a\nb\nc\nd\nx\ne\nf\ng\nh\n"},
	}
	for _, tt := range tests {
		for _, n := range []int{0, 3} {
			t.Run(fmt.Sprintf("%s/context=%d", tt.name, n), func(t *testing.T) {
				checkRoundTrip(t, tt.x, tt.y, n)
			})
		}
	}
}

// checkRoundTrip checks that ParseUnified returns one file with the labels
// and Hunks(x, y) for the diff that Unified writes with n lines of context.
func checkRoundTrip[T string | []byte](t *testing.T, x, y T, n int) {
	t.Helper()
	ctx := diff.Context(n)
	var want []File[T]
	if hunks := Hunks(x, y, ctx); len(hunks) > 0 {
		want = []File[T]{{X: "a/x", Y: "b/y", Hunks: hunks}}
	}
	got, err := ParseUnified(Unified(x, y, ctx, UnifiedHeader("a/x", "b/y")))
	if err != nil {
		t.Fatalf("ParseUnified(...) failed: %v", err)
	}
	// cmp.Diff takes seconds on the large testdata files, so it only runs to
	// report a difference.
	if !reflect.DeepEqual(want, got) {
		t.Errorf("ParseUnified(...) files are different (-want, +got):\n%s", cmp.Diff(want, got))
	}

	// Without UnifiedHeader, the diff is a headerless patch.
	for i := range want {
		want[i].X, want[i].Y = "", ""
	}
	got, err = ParseUnified(Unified(x, y, ctx), ParseHeaderless())
	if err != nil {
		t.Fatalf("ParseUnified(..., ParseHeaderless()) failed: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("ParseUnified(..., ParseHeaderless()) files are different (-want, +got):\n%s", cmp.Diff(want, got))
	}
}

// TestParseUnifiedFiles checks that ParseUnified returns the files of the
// concatenated diffs that Unified writes for the goldens.
func TestParseUnifiedFiles(t *testing.T) {
	var (
		patch []byte
		want  []File[[]byte]
	)
	for i, tt := range parseTests(t) {
		hunks := Hunks(tt.x, tt.y)
		if len(hunks) == 0 {
			continue
		}
		f := File[[]byte]{X: fmt.Sprintf("a/%d", i), Y: fmt.Sprintf("b/%d", i), Hunks: hunks}
		patch = append(patch, Unified(tt.x, tt.y, UnifiedHeader(f.X, f.Y))...)
		want = append(want, f)
	}
	got, err := ParseUnified(patch)
	if err != nil {
		t.Fatalf("ParseUnified(...) failed: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("ParseUnified(...) files are different (-want, +got):\n%s", cmp.Diff(want, got))
	}
}

// TestParseUnifiedHeader checks that a header written with UnifiedHeader
// parses back to the same labels.
func TestParseUnifiedHeader(t *testing.T) {
	labels := []string{
		"",
		"x.txt",
		"a/x y.txt",
		"x y\t.txt",
		"x\t.txt",
		`"a/x\ny"`,
		`"b".txt`,
		"a/x\ny\t\"q\"\\",
		"b/\x00\x1b\x7f\r\a\b\v\fé",
		"\xff\xfe",
	}
	for c := range 256 {
		labels = append(labels, string([]byte{byte(c)}), " "+string([]byte{byte(c)}))
	}
	for _, label := range labels {
		d := Unified("a\n", "b\n", UnifiedHeader(label, "y/"+label))
		files, err := ParseUnified(d)
		if err != nil {
			t.Errorf("ParseUnified(%q) failed: %v", d, err)
			continue
		}
		if len(files) != 1 || files[0].X != label || files[0].Y != "y/"+label {
			t.Errorf("ParseUnified(%q) = %+v, want one file with labels %q, %q", d, files, label, "y/"+label)
		}
	}
}

// TestParseUnifiedCorpus checks ParseUnified on the diffs in testdata/parse,
// written by git diff, GNU diff, BSD diff and by hand. Each diff is parsed in
// the mode of the mode section, "headerless" or "lenient", or in the default mode,
// and either fails with the expected error, or has the expected files and
// turns the x of each file into its y.
func TestParseUnifiedCorpus(t *testing.T) {
	archives, err := filepath.Glob("testdata/parse/*.test")
	if err != nil {
		t.Fatal(err)
	}
	for _, archive := range archives {
		t.Run(strings.TrimSuffix(filepath.Base(archive), ".test"), func(t *testing.T) {
			ar, err := txtar.ParseFile(archive)
			if err != nil {
				t.Fatal(err)
			}
			sections := make(map[string]string)
			for _, f := range ar.Files {
				name, data := f.Name, string(f.Data)
				if n, ok := strings.CutSuffix(name, "-noeol"); ok {
					name, data = n, strings.TrimSuffix(data, "\n")
				}
				sections[name] = data
			}

			d := sections["diff"]
			var opts []ParseOption
			switch mode := strings.TrimSuffix(sections["mode"], "\n"); mode {
			case "":
			case "headerless":
				opts = append(opts, ParseHeaderless())
			case "lenient":
				opts = append(opts, ParseLenient())
			default:
				t.Fatalf("unknown mode %q", mode)
			}
			files, err := ParseUnified(d, opts...)
			filesb, errb := ParseUnified([]byte(d), opts...)
			if fmt.Sprint(errb) != fmt.Sprint(err) || !reflect.DeepEqual(filesb, bytesFiles(files)) {
				t.Errorf("ParseUnified([]byte(diff)) = %+v, %v, want the result of ParseUnified(diff) = %+v, %v", filesb, errb, files, err)
			}

			if len(opts) == 0 {
				checkLenient(t, d, files, err)
			}

			if want, ok := sections["error"]; ok {
				if want = strings.TrimSuffix(want, "\n"); fmt.Sprint(err) != want {
					t.Errorf("ParseUnified(diff) error = %v, want %q", err, want)
				}
				if files != nil {
					t.Errorf("ParseUnified(diff) = %+v with an error, want nil", files)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseUnified(diff) failed: %v", err)
			}
			want, err := parseHeadersSection(sections["headers"])
			if err != nil {
				t.Fatalf("malformed headers section: %v", err)
			}
			var got []File[string]
			for _, f := range files {
				got = append(got, File[string]{X: f.X, Y: f.Y, Extended: f.Extended})
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("ParseUnified(diff) files without hunks are different (-want, +got):\n%s", diff)
			}
			for i, f := range files {
				suffix := ""
				if i > 0 {
					suffix = fmt.Sprint(i + 1)
				}
				got, err := applyHunks(sections["x"+suffix], f.Hunks)
				if err != nil {
					t.Fatalf("applying hunks of file %d failed: %v", i+1, err)
				}
				if diff := cmp.Diff(sections["y"+suffix], got); diff != "" {
					t.Errorf("applying hunks of file %d to x%s yields a different result [-y%s,+got]:\n%s", i+1, suffix, suffix, diff)
				}
			}
		})
	}
}

// parseHeadersSection returns the files of a headers section without hunks.
// Files are separated by blank lines. Each line of a file is "X: ", "Y: " or
// "Extended: " followed by a Go string.
func parseHeadersSection(s string) ([]File[string], error) {
	var files []File[string]
	for block := range strings.SplitSeq(strings.TrimSuffix(s, "\n"), "\n\n") {
		if block == "" {
			continue
		}
		var f File[string]
		var hasX, hasY bool
		for l := range strings.Lines(block) {
			key, q, ok := strings.Cut(strings.TrimSuffix(l, "\n"), ": ")
			if !ok {
				return nil, fmt.Errorf("%q: missing key", l)
			}
			v, err := strconv.Unquote(q)
			if err != nil {
				return nil, fmt.Errorf("%q: %v", l, err)
			}
			switch key {
			case "X":
				f.X, hasX = v, true
			case "Y":
				f.Y, hasY = v, true
			case "Extended":
				f.Extended = append(f.Extended, v)
			default:
				return nil, fmt.Errorf("%q: unknown key", l)
			}
		}
		if !hasX || !hasY {
			return nil, fmt.Errorf("%q: missing X or Y", block)
		}
		files = append(files, f)
	}
	return files, nil
}

// checkLenient checks that ParseUnified in the lenient mode returns files for
// a patch that ParseUnified returns files and no error for.
func checkLenient(t *testing.T, patch string, files []File[string], err error) {
	t.Helper()
	if err != nil {
		return
	}
	if got, err := ParseUnified(patch, ParseLenient()); err != nil || !reflect.DeepEqual(got, files) {
		t.Errorf("ParseUnified(diff, ParseLenient()) = %+v, %v, want %+v", got, err, files)
	}
}

// TestParseUnifiedLenient checks ParseUnified in the lenient mode on patches
// that the default mode rejects.
func TestParseUnifiedLenient(t *testing.T) {
	tests := []struct {
		name   string
		patch  string
		labels []string // Labels of the files, two per file.
		err    string
	}{
		{
			name:   "only-in",
			patch:  "diff -u -r a/f b/f\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+b\nOnly in b: g\ndiff -u -r a/h b/h\n--- a/h\n+++ b/h\n@@ -1 +1 @@\n-c\n+d\n",
			labels: []string{"a/f", "b/f", "a/h", "b/h"},
		},
		{
			name:   "trailing-text",
			patch:  "--- a\n+++ b\n@@ -1 +1 @@\n-a\n+b\n\nthanks\n",
			labels: []string{"a", "b"},
		},
		{
			name:   "text-after-git-header",
			patch:  "diff --git a/f b/f\nold mode 100644\nnew mode 100755\n\ncommit 2\n\ndiff --git a/g b/g\n--- a/g\n+++ b/g\n@@ -1 +1 @@\n-a\n+b\n",
			labels: []string{"", "", "a/g", "b/g"},
		},
		{
			name:   "two-mails",
			patch:  "From 1\n\n---\ndiff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+b\n-- \n2.54\n\nFrom 2\n\n---\ndiff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n-b\n+c\n-- \n2.54\n",
			labels: []string{"a/f", "b/f", "a/f", "b/f"},
		},
		{
			name:   "missing-plus",
			patch:  "--- a\n+++ b\n@@ -1 +1 @@\n-a\n+b\n--- c\ntext\n",
			labels: []string{"a", "b"},
		},
		{
			name:   "overlap-in-y",
			patch:  "--- a\n+++ b\n@@ -1 +1,2 @@\n-a\n+b\n+c\n@@ -3 +2 @@\n-d\n+e\n",
			labels: []string{"a", "b"},
		},
		{
			name:  "overlap-in-x",
			patch: "--- a\n+++ b\n@@ -1,2 +1,2 @@\n-a\n+b\n c\n@@ -2 +3 @@\n-c\n+d\n",
			err:   "line 7: hunk overlaps the previous hunk",
		},
		{
			name:  "hunk-without-header",
			patch: "--- a\n+++ b\n@@ -1 +1 @@\n-a\n+b\ntext\n@@ -3 +3 @@\n-c\n+d\n",
			err:   "line 7: hunk without file header",
		},
		{
			name:  "hunk-after-diff-line",
			patch: "--- a\n+++ b\n@@ -1 +1 @@\n-a\n+b\ndiff -u x y\n@@ -3 +3 @@\n-c\n+d\n",
			err:   `line 7: missing file header after "diff " line`,
		},
		{
			name:   "binary",
			patch:  "--- a\n+++ b\n@@ -1 +1 @@\n-a\n+b\ndiff -r a/x b/x\nBinary files a/x and b/x differ\n",
			labels: []string{"a", "b"},
		},
		{
			name:   "binary-after-hunk",
			patch:  "--- a\n+++ b\n@@ -1 +1 @@\n-a\n+b\nBinary files a/x and b/x differ\n--- c\n+++ d\n@@ -1 +1 @@\n-c\n+d\n",
			labels: []string{"a", "b", "c", "d"},
		},
		{
			name:  "binary-first",
			patch: "Binary files a/x and b/x differ\n--- a\n+++ b\n@@ -1 +1 @@\n-a\n+b\n",
			err:   "line 1: binary diff",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, err := ParseUnified(tt.patch, ParseLenient())
			if tt.err != "" {
				if fmt.Sprint(err) != tt.err {
					t.Errorf("ParseUnified(...) error = %v, want %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseUnified(...) failed: %v", err)
			}
			var labels []string
			for _, f := range files {
				labels = append(labels, f.X, f.Y)
			}
			if !slices.Equal(labels, tt.labels) {
				t.Errorf("ParseUnified(...) labels = %q, want %q", labels, tt.labels)
			}
		})
	}
}

// TestParseUnifiedCopies checks that the labels and extended header lines
// parsed from a []byte patch do not change when the patch does, and that the
// lines of the edits do.
func TestParseUnifiedCopies(t *testing.T) {
	patch := []byte("diff --git a/x b/x\nindex 1..2\n--- old\t2026-01-02\n+++ \"new\"\n@@ -1 +1 @@\n-a\n+b\n")
	files, err := ParseUnified(patch)
	if err != nil || len(files) != 1 {
		t.Fatalf("ParseUnified(...) = %+v, %v, want one file", files, err)
	}
	for i := range patch {
		patch[i] = 'X'
	}
	f := files[0]
	if want := []string{"diff --git a/x b/x", "index 1..2"}; f.X != "old" || f.Y != "new" || !slices.Equal(f.Extended, want) {
		t.Errorf("file = %q, %q, %q after changing the patch, want %q, %q, %q", f.X, f.Y, f.Extended, "old", "new", want)
	}
	if got := string(f.Hunks[0].Edits[0].Line); got != "XX" {
		t.Errorf("first line = %q after changing the patch, want %q", got, "XX")
	}
}

// TestParseUnifiedAllocation checks that a malformed patch does not allocate
// memory for more edits than it has lines.
func TestParseUnifiedAllocation(t *testing.T) {
	for _, patch := range []string{
		"@@ -1 +1 @@\n" + strings.Repeat("\n", 1<<20),
		"@@ -1,1000000000 +1,1000000000 @@\n-a\n",
	} {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		if _, err := ParseUnified(patch, ParseHeaderless()); err == nil {
			t.Fatalf("ParseUnified(%.40q) succeeded, want an error", patch)
		}
		runtime.ReadMemStats(&after)
		if n := after.TotalAlloc - before.TotalAlloc; n > 1<<16 {
			t.Errorf("ParseUnified(%.40q) allocated %d bytes, want at most %d", patch, n, 1<<16)
		}
	}
}

func FuzzParseUnified(f *testing.F) {
	f.Add("")
	f.Add("--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n a\n-b\n+c\n")
	f.Add("@@ -0,0 +1 @@\n+a\n\\ No newline at end of file\n")
	f.Add("@@ -1,3 +1,2 @@ text\n a\n\n-b\n@@ -9 +8,0 @@\n-c\n")
	f.Add("--- \"a\\tb\\303\"\tdate\n+++ b\n")
	f.Add("--- a\n+++ b\n@@ -1 +1 @@\n-a\n+b\nOnly in b: g\ndiff -u a/h b/h\n--- a/h\n+++ b/h\n@@ -1 +1 @@\n-c\n+d\n")
	f.Add("diff --git a/f b/f\nold mode 100644\nnew mode 100755\n\n-- \nFrom 2\n--- c\n")
	f.Fuzz(func(t *testing.T, patch string) {
		files, err := ParseUnified(patch)
		lenient, lerr := ParseUnified(patch, ParseLenient())
		if err == nil && (lerr != nil || !reflect.DeepEqual(files, lenient)) {
			t.Fatalf("ParseUnified(patch, ParseLenient()) = %+v, %v, want %+v", lenient, lerr, files)
		}
		headerless, _ := ParseUnified(patch, ParseHeaderless())
		if len(headerless) > 1 || len(headerless) == 1 && (headerless[0].X != "" || headerless[0].Y != "" || headerless[0].Extended != nil) {
			t.Fatalf("ParseUnified(patch, ParseHeaderless()) = %+v, want at most one file without a header", headerless)
		}
		for _, f := range slices.Concat(lenient, headerless) {
			for _, h := range f.Hunks {
				x, y := h.LineNoX, h.LineNoY
				for _, e := range h.Edits {
					if e.Op != diff.Insert {
						if e.LineNoX != x {
							t.Fatalf("edit %+v has LineNoX %d, want %d", e, e.LineNoX, x)
						}
						x++
					}
					if e.Op != diff.Delete {
						if e.LineNoY != y {
							t.Fatalf("edit %+v has LineNoY %d, want %d", e, e.LineNoY, y)
						}
						y++
					}
				}
				if x != h.EndLineNoX || y != h.EndLineNoY {
					t.Fatalf("hunk %+v ends at %d, %d", h, x, y)
				}
			}
		}
	})
}

// BenchmarkParseUnified runs ParseUnified on the output of Unified for the
// inputs of package benchdata. The full variants use a context larger than
// the input, so every line is a body line. The mixed variant parses the diffs
// of all inputs of package textdiff and internal/benchmarks.
func BenchmarkParseUnified(b *testing.B) {
	for _, name := range benchdata.Names {
		for _, c := range []struct {
			mode string
			opts []Option
		}{
			{"default", nil},
			{"full", []Option{diff.Context(1 << 30)}},
		} {
			b.Run(name+"/"+c.mode, func(b *testing.B) {
				x, y := benchdata.Load(b, name)
				patch := Unified(x, y, c.opts...)
				b.SetBytes(int64(len(patch)))
				b.ReportAllocs()
				for b.Loop() {
					if _, err := ParseUnified(patch, ParseHeaderless()); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
	b.Run("mixed", func(b *testing.B) {
		var patches [][]byte
		var n int64
		for _, p := range benchdata.All(b, 1<<30) {
			patch := Unified(p.X, p.Y)
			patches = append(patches, patch)
			n += int64(len(patch))
		}
		b.SetBytes(n)
		b.ReportAllocs()
		for b.Loop() {
			for _, patch := range patches {
				if _, err := ParseUnified(patch, ParseHeaderless()); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

// bytesFiles returns files with []byte lines.
func bytesFiles(files []File[string]) []File[[]byte] {
	var out []File[[]byte]
	for _, f := range files {
		fb := File[[]byte]{X: f.X, Y: f.Y, Extended: f.Extended}
		for _, h := range f.Hunks {
			hb := Hunk[[]byte]{LineNoX: h.LineNoX, EndLineNoX: h.EndLineNoX, LineNoY: h.LineNoY, EndLineNoY: h.EndLineNoY}
			for _, e := range h.Edits {
				hb.Edits = append(hb.Edits, Edit[[]byte]{e.Op, e.LineNoX, e.LineNoY, []byte(e.Line)})
			}
			fb.Hunks = append(fb.Hunks, hb)
		}
		out = append(out, fb)
	}
	return out
}

// applyHunks returns x with hunks applied at their exact positions.
func applyHunks(x string, hunks []Hunk[string]) (string, error) {
	xl := slices.Collect(strings.Lines(x))
	var b strings.Builder
	pos := 0
	for _, h := range hunks {
		b.WriteString(strings.Join(xl[pos:h.LineNoX], ""))
		for _, e := range h.Edits {
			if e.Op != diff.Insert && xl[e.LineNoX] != e.Line {
				return "", fmt.Errorf("line %d is %q, hunk has %q", e.LineNoX, xl[e.LineNoX], e.Line)
			}
			if e.Op != diff.Delete {
				b.WriteString(e.Line)
			}
		}
		pos = h.EndLineNoX
	}
	b.WriteString(strings.Join(xl[pos:], ""))
	return b.String(), nil
}
