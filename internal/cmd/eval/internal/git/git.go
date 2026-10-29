// Copyright 2025 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package git provides a simplified git interface for reading a repository for
// evaluations
package git

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"znkr.io/diff/textdiff"
)

type Repo struct {
	dir    string
	gitcat chan<- gitcatterinstr
	done   chan struct{}
}

func Open(dir string) (*Repo, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, err
	}

	gitcat, done := gitcatter(dir)

	return &Repo{
		dir:    dir,
		gitcat: gitcat,
		done:   done,
	}, nil
}

func (r *Repo) Close() {
	close(r.gitcat)
	<-r.done
}

func (r *Repo) RevList() ([]string, error) {
	out, err := git("-C", r.dir, "rev-list", "--no-merges", "HEAD")
	if err != nil {
		return nil, err
	}
	revs := strings.Split(out, "\n")
	if revs[len(revs)-1] == "" {
		revs = revs[:len(revs)-1]
	}
	return revs, nil
}

type FileDiff struct {
	Name  string
	OldID string
	NewID string
	// Hunks of git's diffs of the file, in the order they apply. A type
	// change, such as a file that becomes a symlink, is a deletion and an
	// addition.
	Patches [][]textdiff.Hunk[string]
}

// DiffTree returns the files that commit changes, without submodules, and the
// hunks of git's diff of each file. Binary files are diffed as text.
func (r *Repo) DiffTree(commit string) ([]FileDiff, error) {
	out, err := git("-C", r.dir, "diff-tree", "-r", "--raw", "-p", "-z", "--full-index", "--text", commit)
	if err != nil {
		return nil, err
	}
	// With -z, the output is the commit ID, a raw line and a path for each
	// file, and the patch. Each field ends in a NUL, and an empty field ends
	// the raw lines:
	//
	//	<commit>\0
	//	:100644 100644 <old id> <new id> M\0<path>\0
	//	...
	//	\0
	//	diff --git a/<path> b/<path>
	//	index <old id>..<new id> 100644
	//	...
	raw, out, _ := strings.Cut(out, "\x00\x00")
	patch, err := textdiff.ParseUnified(out)
	if err != nil {
		return nil, fmt.Errorf("parsing diff-tree patch: %v", err)
	}
	// The index line of a file's diff names the same blob IDs as its raw line.
	hunks := make(map[string][]textdiff.Hunk[string]) // Keyed by "<old id>..<new id>".
	for _, f := range patch {
		for _, l := range f.Extended {
			if rest, ok := strings.CutPrefix(l, "index "); ok {
				ids, _, _ := strings.Cut(rest, " ")
				hunks[ids] = f.Hunks
			}
		}
	}
	fields := strings.Split(raw, "\x00")[1:] // Without the commit ID.
	if len(fields)%2 != 0 {
		return nil, fmt.Errorf("diff-tree raw output has a line without a path: %q", raw)
	}
	var ret []FileDiff
	for i := 0; i < len(fields); i += 2 {
		line, name := fields[i], fields[i+1]
		if !strings.HasPrefix(line, ":") {
			return nil, fmt.Errorf("diff-tree file not starting with ':': %q", line)
		}
		// f is old mode, new mode, old ID, new ID and status. Mode 160000 is
		// a submodule, a commit that this repository doesn't contain.
		f := strings.Fields(line[1:])
		if f[0] == "160000" || f[1] == "160000" {
			continue
		}
		old, new := f[2], f[3]
		patches := [][]textdiff.Hunk[string]{hunks[old+".."+new]}
		if f[4] == "T" {
			// git writes a type change as two diffs: old ID to the zero ID,
			// and the zero ID to new ID.
			zero := strings.Repeat("0", len(old))
			patches = [][]textdiff.Hunk[string]{hunks[old+".."+zero], hunks[zero+".."+new]}
		}
		ret = append(ret, FileDiff{
			Name:    name,
			OldID:   old,
			NewID:   new,
			Patches: patches,
		})
	}
	return ret, nil
}

func (r *Repo) Read(blobIDs []string, cb func([]string)) {
	r.gitcat <- gitcatterinstr{blobIDs, cb}
}

func git(args ...string) (string, error) {
	var wout, werr strings.Builder
	cmd := exec.Command("git", args...)
	cmd.Stdout = &wout
	cmd.Stderr = &werr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("running git command %v: %v\n%s", cmd, err, werr.String())
	}
	return wout.String(), nil
}

type gitcatterinstr struct {
	blobIds []string
	cb      func([]string)
}

func gitcatter(repo string) (chan<- gitcatterinstr, chan struct{}) {
	wc := make(chan gitcatterinstr)
	rc := make(chan []gitcatterinstr, runtime.GOMAXPROCS(0))
	done := make(chan struct{})

	cmd := exec.Command("git", "-C", repo, "cat-file", "--batch-command", "--buffer")
	in, err := cmd.StdinPipe()
	if err != nil {
		panic(fmt.Sprintf("failed to connect stdin: %v", err))
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		panic(fmt.Sprintf("failed to connect stdout: %v", err))
	}
	var werr bytes.Buffer
	cmd.Stderr = &werr
	if err := cmd.Start(); err != nil {
		panic(err)
	}

	r, w := bufio.NewReader(out), in
	go func() {
		defer close(rc)
		const N = 32
		for {
			bundle := make([]gitcatterinstr, 0, N)
		Write:
			for range N {
				select {
				case instr, ok := <-wc:
					if !ok {
						return
					}
					for _, id := range instr.blobIds {
						if id == "0000000000000000000000000000000000000000" {
							continue
						}
						if _, err := fmt.Fprintf(w, "contents %s\n", id); err != nil {
							panic(fmt.Sprintf("writing to stdin pipe: %v", err))
						}
					}
					bundle = append(bundle, instr)
				default:
					break Write
				}
			}

			if _, err := fmt.Fprintf(w, "flush\n"); err != nil {
				panic(fmt.Sprintf("writing to stdin pipe: %v", err))
			}
			rc <- bundle
		}
	}()

	go func() {
		defer close(done)
		for bundle := range rc {
			for _, instr := range bundle {
				out := make([]string, len(instr.blobIds))
				for i, id := range instr.blobIds {
					if id == "0000000000000000000000000000000000000000" {
						continue
					}
					line, err := r.ReadString('\n')
					if err != nil {
						panic(err)
					}
					fields := strings.Fields(line)
					if len(fields) != 3 {
						panic(fmt.Sprintf("found %v fields, expected 3: %q", len(fields), line))
					}
					if fields[0] != id {
						panic(fmt.Sprintf("ids don't match %s vs %s", fields[0], id))
					}
					n, err := strconv.ParseInt(fields[2], 10, 64)
					if err != nil {
						panic(err)
					}
					buf := make([]byte, n+1)
					if _, err := io.ReadFull(r, buf); err != nil {
						panic(err)
					}
					out[i] = string(buf[:n])
				}

				instr.cb(out)
			}
		}
	}()

	return wc, done
}
