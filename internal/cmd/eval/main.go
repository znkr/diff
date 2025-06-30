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

// eval provides a way to validate the diffing algorithm by applying the resulting diffs using
// the unix patch tool and checking that they produce the input again.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"znkr.io/diff/internal/cmd/eval/internal/git"
	"znkr.io/diff/internal/unixpatch"
	"znkr.io/diff/textdiff"
)

type config struct {
	repo string
}

func main() {
	var cfg config
	flag.StringVar(&cfg.repo, "repo", "", "repository to use for evaluation")
	flag.Parse()

	if err := run(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
}

var bars = []string{
	" ",
	"▏",
	"▎",
	"▍",
	"▌",
	"▋",
	"▊",
	"▉",
	"█",
}

type note struct {
	prefix string
	msg    string
}

func run(cfg *config) error {
	start := time.Now()
	notes := make(chan note)
	done := make(chan struct{})
	var commitsDone atomic.Int64
	var diffsDone atomic.Int64

	git, err := git.Open(cfg.repo)
	if err != nil {
		return fmt.Errorf("opening git repository: %v", err)
	}

	commitIDs, err := git.RevList()
	if err != nil {
		return fmt.Errorf("reading rev-list: %v", err)
	}

	// Process commits.
	type change struct {
		commitID string
		filename string
		old, new string
	}
	changes := make(chan change)
	var changesWG sync.WaitGroup
	chunkSize := len(commitIDs) / (4 * runtime.GOMAXPROCS(0))
	for chunk := range slices.Chunk(commitIDs, chunkSize) {
		changesWG.Add(1)
		go func() {
			defer changesWG.Done()
			for _, commitID := range chunk {

				files, err := git.DiffTree(commitID)
				if err != nil {
					notes <- note{
						prefix: commitID,
						msg:    fmt.Sprintf("error proccesing commit: %v", err),
					}
				}
				for _, file := range files {
					if strings.HasSuffix(file.Name, ".zip") || strings.HasSuffix(file.Name, ".syso") {
						continue
					}
					git.Read([]string{file.OldID, file.NewID}, func(res []string) {
						changes <- change{
							commitID: commitID,
							filename: file.Name,
							old:      res[0],
							new:      res[1],
						}
					})
				}
				commitsDone.Add(1)
			}
		}()
	}

	// Process diffs.
	var diffWG sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		diffWG.Add(1)
		go func() {
			defer diffWG.Done()
			for change := range changes {
				diff := textdiff.Unified(change.old, change.new, textdiff.IndentHeuristic())
				patched, err := unixpatch.Patch(change.old, diff)
				if err != nil {
					notes <- note{
						prefix: change.commitID + ":" + change.filename,
						msg:    fmt.Sprintf("failed to run patch: %v", err),
					}
				}
				if change.new != patched {
					notes <- note{
						prefix: change.commitID + ":" + change.filename,
						msg:    fmt.Sprintf("file is different after applying patch. got:\n%s\nwant:\n%s", change.new, patched),
					}
				}
				diffsDone.Add(1)
			}
		}()
	}

	// Render progress
	var progressWG sync.WaitGroup
	render := func() {
		const width = 60
		commits := commitsDone.Load()
		diffs := diffsDone.Load()
		progress := float64(commits) / float64(len(commitIDs))
		whole := int(progress * width)
		remainder := math.Mod(progress*width, 1)
		last := bars[max(0, min(len(bars), int(remainder*float64(len(bars)))))]
		if width-whole < 1 {
			last = ""
		}
		bar := strings.Repeat(bars[len(bars)-1], whole) + last
		var commitsPerSec, diffsPerSec int
		if commits > 0 {
			commitsPerSec = int((time.Duration(commits) * time.Second) / time.Since(start))
		}
		if diffs > 0 {
			diffsPerSec = int((time.Duration(diffs) * time.Second) / time.Since(start))
		}
		fmt.Printf("\r[%-*s] % 3.1f%% (%d commits/s, %d diff/s) ", width, bar, 100*progress, commitsPerSec, diffsPerSec)
	}
	progressWG.Add(1)
	go func() {
		defer progressWG.Done()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case note := <-notes:
				fmt.Printf("\r%s: %s\n", note.prefix, note.msg)
				render()

			case <-ticker.C:
				render()

			case <-done:
				render()
				fmt.Printf("\n")
				return
			}
		}
	}()

	// Shutdown
	changesWG.Wait()
	git.Close()
	close(changes)
	diffWG.Wait()
	close(done)
	progressWG.Wait()

	return nil
}
