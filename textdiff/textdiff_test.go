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
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/tools/txtar"
	"znkr.io/diff"
	"znkr.io/diff/internal/config"
	"znkr.io/diff/internal/unixpatch"
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
					if diff := cmp.Diff(st.want, got); diff != "" {
						t.Errorf("UnifiedBytes(...) result are different:\ngot:\n%s\nwant:\n%s\ndiff [-got,+want]:\n%s", got, st.want, diff)
					}
					if *validate && len(got) > 0 {
						patched, err := unixpatch.Patch(string(tt.x), string(got))
						if err != nil {
							t.Fatalf("failed to run patch: %v", err)
						}
						if diff := cmp.Diff(tt.y, []byte(patched)); diff != "" {
							t.Errorf("file is different after applying patch [-got,+want]:\n%s", diff)
						}
					}
					if *update {
						tt.subtests[sti].want = got
					}
				})
			}

			// Run in a cleanup to makes sure to runs after the subtests have finished.
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
			want: "@@ -1,0 +1,1 @@\n+one-line\n",
		},
		{
			name: "y-empty",
			x:    "one-line\n",
			y:    "",
			want: "@@ -1,1 +1,0 @@\n-one-line\n",
		},
		{
			name: "missing-newline-x",
			x:    "first line",
			y:    "first line\n",
			want: "@@ -1,1 +1,1 @@\n-first line\n\\ No newline at end of file\n+first line\n",
		},
		{
			name: "missing-newline-y",
			x:    "first line\n",
			y:    "first line",
			want: "@@ -1,1 +1,1 @@\n-first line\n+first line\n\\ No newline at end of file\n",
		},
		{
			name: "missing-newline-both",
			x:    "a\nsecond line",
			y:    "b\nsecond line",
			want: "@@ -1,2 +1,2 @@\n-a\n+b\n second line\n\\ No newline at end of file\n",
		},
		{
			name: "missing-newline-empty-x",
			x:    "",
			y:    "\n",
			want: "@@ -1,0 +1,1 @@\n+\n", // no missing newline note here
		},
		{
			name: "missing-newline-empty-y",
			x:    "\n",
			y:    "",
			want: "@@ -1,1 +1,0 @@\n-\n", // no missing newline note here
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unified(tt.x, tt.y)
			if got != tt.want {
				t.Errorf("Unified(...) if different:\ngot:  %q\nwant: %q", got, tt.want)
			}
			if *validate && len(got) > 0 {
				patched, err := unixpatch.Patch(tt.x, got)
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
							// The inline function definition is necessary, because the anchoring
							// heuristic is not exported as an option.
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
