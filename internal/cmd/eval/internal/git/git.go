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

// Package git provides a simplified git interface for reading a repository for evaluations
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
}

func (r *Repo) DiffTree(commit string) ([]FileDiff, error) {
	out, err := git("-C", r.dir, "diff-tree", "-r", commit)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(out, "\n")[1:]
	ret := make([]FileDiff, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if line[0] != ':' {
			return nil, fmt.Errorf("diff-tree file not starting with ':': %q", line)
		}
		fields := strings.Fields(line[1:])
		ret = append(ret, FileDiff{
			Name:  fields[5],
			OldID: fields[2],
			NewID: fields[3],
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
