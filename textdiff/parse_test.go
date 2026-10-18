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

// checkRoundTrip checks that ParseUnified returns Hunks(x, y) and the header
// for the diff that Unified writes with n lines of context.
func checkRoundTrip[T string | []byte](t *testing.T, x, y T, n int) {
	t.Helper()
	ctx := diff.Context(n)
	want := Hunks(x, y, ctx)
	h, got, err := ParseUnified(Unified(x, y, ctx, UnifiedHeader("a/x", "b/y")))
	if err != nil {
		t.Fatalf("ParseUnified(...) failed: %v", err)
	}
	// cmp.Diff takes seconds on the large testdata files, so it only runs to
	// report a difference.
	if !reflect.DeepEqual(want, got) {
		t.Errorf("ParseUnified(...) hunks are different (-want, +got):\n%s", cmp.Diff(want, got))
	}
	switch wantHeader := (Header{"a/x", "b/y"}); {
	case len(got) > 0 && (h == nil || *h != wantHeader):
		t.Errorf("ParseUnified(...) header = %+v, want %+v", h, wantHeader)
	case len(got) == 0 && h != nil:
		t.Errorf("ParseUnified(\"\") header = %+v, want nil", h)
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
		h, _, err := ParseUnified(d)
		if err != nil {
			t.Errorf("ParseUnified(%q) failed: %v", d, err)
			continue
		}
		if want := (Header{label, "y/" + label}); h == nil || *h != want {
			t.Errorf("ParseUnified(%q) header = %q, want %q", d, h, want)
		}
	}
}

// TestParseUnifiedCorpus checks ParseUnified on the diffs in testdata/parse,
// written by git diff, GNU diff, BSD diff and by hand. Each diff either fails
// with the expected error, or has the expected header and turns x into y.
func TestParseUnifiedCorpus(t *testing.T) {
	files, err := filepath.Glob("testdata/parse/*.test")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		t.Run(strings.TrimSuffix(filepath.Base(file), ".test"), func(t *testing.T) {
			ar, err := txtar.ParseFile(file)
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
			h, hunks, err := ParseUnified(d)
			hb, hunksb, errb := ParseUnified([]byte(d))
			if fmt.Sprint(errb) != fmt.Sprint(err) || !reflect.DeepEqual(hb, h) || !reflect.DeepEqual(hunksb, bytesHunks(hunks)) {
				t.Errorf("ParseUnified([]byte(diff)) = %q, %v, %v, want the result of ParseUnified(diff) = %q, %v, %v", hb, hunksb, errb, h, hunks, err)
			}

			if want, ok := sections["error"]; ok {
				if want = strings.TrimSuffix(want, "\n"); fmt.Sprint(err) != want {
					t.Errorf("ParseUnified(diff) error = %v, want %q", err, want)
				}
				if h != nil {
					t.Errorf("ParseUnified(diff) header = %q with an error, want nil", *h)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseUnified(diff) failed: %v", err)
			}
			var want *Header
			if labels, ok := sections["header"]; ok {
				want = new(Header)
				if _, err := fmt.Sscanf(labels, "%q\n%q\n", &want.X, &want.Y); err != nil {
					t.Fatalf("malformed header section: %v", err)
				}
			}
			if !reflect.DeepEqual(h, want) {
				t.Errorf("ParseUnified(diff) header = %+v, want %+v", h, want)
			}
			got, err := applyHunks(sections["x"], hunks)
			if err != nil {
				t.Fatalf("applying hunks failed: %v", err)
			}
			if diff := cmp.Diff(sections["y"], got); diff != "" {
				t.Errorf("applying hunks to x yields a different result [-y,+got]:\n%s", diff)
			}
		})
	}
}

// TestParseUnifiedLabelsCopied checks that the labels parsed from a []byte
// patch do not change when the patch does.
func TestParseUnifiedLabelsCopied(t *testing.T) {
	patch := []byte("--- old\t2026-01-02\n+++ \"new\"\n@@ -1 +1 @@\n-a\n+b\n")
	h, _, err := ParseUnified(patch)
	if err != nil || h == nil {
		t.Fatalf("ParseUnified(...) = %v, _, %v, want a header", h, err)
	}
	for i := range patch {
		patch[i] = 'X'
	}
	if want := (Header{"old", "new"}); *h != want {
		t.Errorf("header = %q after changing the patch, want %q", *h, want)
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
		if _, _, err := ParseUnified(patch); err == nil {
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
	f.Fuzz(func(t *testing.T, patch string) {
		_, hunks, err := ParseUnified(patch)
		if err != nil {
			return
		}
		for _, h := range hunks {
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
					if _, _, err := ParseUnified(patch); err != nil {
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
				if _, _, err := ParseUnified(patch); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

// bytesHunks returns hunks with []byte lines.
func bytesHunks(hunks []Hunk[string]) []Hunk[[]byte] {
	var out []Hunk[[]byte]
	for _, h := range hunks {
		hb := Hunk[[]byte]{LineNoX: h.LineNoX, EndLineNoX: h.EndLineNoX, LineNoY: h.LineNoY, EndLineNoY: h.EndLineNoY}
		for _, e := range h.Edits {
			hb.Edits = append(hb.Edits, Edit[[]byte]{e.Op, e.LineNoX, e.LineNoY, []byte(e.Line)})
		}
		out = append(out, hb)
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
