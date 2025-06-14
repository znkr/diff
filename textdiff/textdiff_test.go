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
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"
	"znkr.io/diff"
)

var update = flag.Bool("update", false, "update golden files")
var exhaustive = flag.Bool("exhaustive", false, "perform the exhaustive test")

func TestUnified(t *testing.T) {
	type subtest struct {
		opts map[string]string
		want []byte
	}

	tests, err := filepath.Glob("testdata/*.test")
	if err != nil {
		t.Fatalf("Failed to read testdata: %v", err)
	}
	for _, test := range tests {
		name := strings.TrimPrefix(test, "testdata/")
		t.Run(name, func(t *testing.T) {
			ar, err := txtar.ParseFile(test)
			if err != nil {
				t.Fatalf("failed to parse test case: %v", err)
			}

			var x, y []byte
			var subtests []subtest
			for _, f := range ar.Files {
				switch f.Name {
				case "x":
					x = f.Data
				case "y":
					y = f.Data
				case "diff":
					data := f.Data
					var st subtest
					for i := 0; i < len(data); i++ {
						if data[i] != '#' {
							st.want = data[i:]
							break
						}
						i++
						eol := i + bytes.IndexByte(data[i:], '\n')
						if eol < i {
							panic("missing newline after option line")
						}
						k, v, found := bytes.Cut(data[i:eol], []byte{':'})
						if !found {
							panic("missing : in option line")
						}
						if st.opts == nil {
							st.opts = make(map[string]string)
						}
						st.opts[string(bytes.TrimSpace(k))] = string(bytes.TrimSpace(v))
						i = eol
					}
					subtests = append(subtests, st)
				default:
					t.Fatalf("unknown file in archive: %v", f)
				}
			}

			for i, st := range subtests {
				var name strings.Builder
				for i, k := range slices.Sorted(maps.Keys(st.opts)) {
					if i > 0 {
						name.WriteString(":")
					}
					name.WriteString(k)
					name.WriteString("=")
					name.WriteString(st.opts[k])
				}
				if len(st.opts) == 0 {
					name.WriteString("default")
				}
				t.Run(name.String(), func(t *testing.T) {
					var opts []diff.Option
					for k, v := range st.opts {
						switch k {
						case "indent-heuristic":
							switch v {
							case "true":
								opts = append(opts, IndentHeuristic())
							case "false":
								// do nothing
							default:
								panic("invalid value for indent_heuristic: " + v)
							}
						case "context":
							n, err := strconv.ParseInt(v, 10, 64)
							if err != nil {
								panic("invalid value for context: " + err.Error())
							}
							opts = append(opts, diff.Context(int(n)))
						default:
							panic("unknown option: " + k)
						}
					}

					got := Unified(x, y, opts...)
					if !bytes.Equal(got, st.want) {
						t.Errorf("UnifiedBytes(...) result are different:\ngot:\n%s\nwant:\n%s", got, st.want)
					}
					if *update {
						subtests[i].want = got
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

				write(ar.Comment)
				write([]byte("-- x --\n"))
				write(x)
				write([]byte("-- y --\n"))
				write(y)
				for _, st := range subtests {
					write([]byte("-- diff --\n"))
					for _, k := range slices.Sorted(maps.Keys(st.opts)) {
						write([]byte("# " + k + ": " + st.opts[k] + "\n"))
					}
					write(st.want)
				}

				if err := f.Close(); err != nil {
					t.Fatalf("error closing golden file: %v", err)
				}
				if err := os.Rename(f.Name(), test); err != nil {
					t.Fatalf("error renaming golden file: %v", err)
				}
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
