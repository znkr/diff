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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"
	"znkr.io/diff"
	"znkr.io/diff/internal/config"
)

var update = flag.Bool("update", false, "update golden files")
var exhaustive = flag.Bool("exhaustive", false, "perform the exhaustive test")

func TestUnified(t *testing.T) {
	for _, tt := range parseTests(t) {
		t.Run(tt.name, func(t *testing.T) {
			for sti, st := range tt.subtests {
				t.Run(st.name, func(t *testing.T) {
					got := Unified(tt.x, tt.y, st.opts...)
					if !bytes.Equal(got, st.want) {
						t.Errorf("UnifiedBytes(...) result are different:\ngot:\n%s\nwant:\n%s", got, st.want)
					}
					if *update {
						tt.subtests[sti].want = got
					}
				})
			}

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
	}
}

func TestUnifiedAllocs(t *testing.T) {
	for _, tt := range parseTests(t) {
		t.Run(tt.name, func(t *testing.T) {
			for _, st := range tt.subtests {
				t.Run(st.name, func(t *testing.T) {
					allocs := testing.AllocsPerRun(10, func() {
						_ = Unified(tt.x, tt.y, st.opts...)
					})
					if allocs > 7 {
						t.Errorf("Number of allocations in Edits was %v, want <= %v", allocs, 7)
					}
				})
			}
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
		test := test{
			name:     strings.TrimPrefix(filename, "testdata/"),
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
				for i := 0; i < len(data); i++ {
					if data[i] != '#' {
						st.pragmas = data[:i]
						st.want = data[i:]
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
							t.Fatalf("invalid value for indent_heuristic: %q", v)
						}
						name = append(name, k)
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
				test.subtests = append(test.subtests, st)
			default:
				t.Fatalf("unknown file in archive: %v", f)
			}
		}
		tests = append(tests, test)
	}
	return tests
}

func TestUnifiedExhaustive(t *testing.T) {
	if !*exhaustive {
		t.Skip("exhaustive test not required")
	}

	tests := []struct {
		name string
		repo string
	}{
		{
			name: "go",
			repo: "https://go.googlesource.com/go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := "corpus/" + tt.name + ".git"
			if _, err := os.Stat(repo); os.IsNotExist(err) {
				git(t, "clone", "--quiet", "--bare", tt.repo, repo)
			} else {
				git(t, "-C", repo, "fetch", "--quiet")
			}

			commitIDs := git(t, "-C", repo, "rev-list", "--no-merges", "HEAD")
			for commitID := range bytes.Lines(commitIDs) {
				commitID = commitID[:len(commitID)-1] // strip trailing newline
				t.Run(string(commitID), func(t *testing.T) {
					treeDiff := git(t, "-C", repo, "diff-tree", "-r", string(commitID))
					files := bytes.Split(treeDiff, []byte("\n"))[1:]
					for _, file := range files {
						if len(file) == 0 {
							continue
						}
						if file[0] != ':' {
							t.Fatalf("not starting with ':': %s", file)
						}
						fields := bytes.Fields(file[1:])
						oldBlobID := fields[2]
						newBlobID := fields[3]
						filename := fields[5]

						var old, new []byte
						if !bytes.Equal(oldBlobID, []byte("0000000000000000000000000000000000000000")) {
							old = git(t, "-C", repo, "cat-file", "blob", string(oldBlobID))
						}
						if !bytes.Equal(newBlobID, []byte("0000000000000000000000000000000000000000")) {
							new = git(t, "-C", repo, "cat-file", "blob", string(newBlobID))
						}

						testname := strings.ReplaceAll(string(filename), "/", "_")
						t.Run(testname, func(t *testing.T) {
							defer func() {
								if p := recover(); p != nil {
									var buf bytes.Buffer
									fmt.Fprintf(&buf, "From %s\ncommit %s\nfile %s\n", tt.repo, commitID, filename)
									buf.WriteString("-- x --\n")
									buf.Write(old)
									buf.WriteString("-- y --\n")
									buf.Write(new)
									buf.WriteString("-- diff --\n")
									buf.WriteString("-- diff --\n# indent-heuristic: true\n")
									reprofile := "testdata/" + tt.name + "_" + string(commitID) + "_" + testname + ".test"
									err := os.WriteFile(reprofile, buf.Bytes(), 0o644)
									if err != nil {
										t.Errorf("failed to write reproducer: %v", err)
									}
									panic(p)
								}
							}()
							Unified(old, new, IndentHeuristic())
						})
					}
				})
			}
		})
	}
}

func git(t *testing.T, args ...string) []byte {
	t.Helper()
	var wout, werr bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Stdout = &wout
	cmd.Stderr = &werr
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run git command: git %s: %v\n%s", strings.Join(args, " "), err, werr.Bytes())
	}
	return wout.Bytes()
}
