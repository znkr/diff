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
	"bufio"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"znkr.io/diff"
	"znkr.io/diff/internal/cmd/eval/internal/git"
	"znkr.io/diff/internal/unixpatch"
	"znkr.io/diff/textdiff"
)

type config struct {
	repo     string
	sample   int
	parallel int
	stats    string
	validate bool
}

func main() {
	var cfg config
	flag.StringVar(&cfg.repo, "repo", "", "repository to use for evaluation")
	flag.IntVar(&cfg.sample, "sample", 0, "if >0, sample commits to the value of the flag")
	flag.IntVar(&cfg.parallel, "parallel", runtime.GOMAXPROCS(0), "number of evaluations to run in parallel")
	flag.StringVar(&cfg.stats, "stats", "", "file to store stats in")
	flag.BoolVar(&cfg.validate, "validate", true, "if validation should be performed")
	flag.Parse()

	if len(flag.CommandLine.Args()) > 0 {
		fmt.Fprintf(os.Stderr, "error: unexpected command line arguments: %v\n", flag.CommandLine.Args())
		os.Exit(1)
	}

	if err := run(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
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

type result struct {
	commitID string
	file     string
	variant  string
	N, M     int
	D        int
	duration time.Duration
}

func run(cfg *config) error {
	start := time.Now()
	notes := make(chan note)
	done := make(chan struct{})
	var commitsDone atomic.Int64
	var processed atomic.Int64

	var stats *os.File
	if cfg.stats != "" {
		var err error
		stats, err = os.Create(cfg.stats)
		if err != nil {
			return fmt.Errorf("creating stats file: %v", err)
		}
	}

	git, err := git.Open(cfg.repo)
	if err != nil {
		return fmt.Errorf("opening git repository: %v", err)
	}

	commitIDs, err := git.RevList()
	if err != nil {
		return fmt.Errorf("reading rev-list: %v", err)
	}

	// Sample commits
	if cfg.sample > 0 && cfg.sample < len(commitIDs) {
		picked := make(map[int]struct{}, cfg.sample)
		sample := make([]string, 0, cfg.sample)
		for len(sample) < cfg.sample {
			i := rand.IntN(len(commitIDs))
			if _, ok := picked[i]; ok {
				continue
			}
			sample = append(sample, commitIDs[i])
			picked[i] = struct{}{}
		}
		commitIDs = sample
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
	var processWG sync.WaitGroup
	var results chan result
	if cfg.stats != "" {
		results = make(chan result)
	}
	for range cfg.parallel {
		processWG.Add(1)
		go func() {
			defer processWG.Done()
			for change := range changes {
				variants := map[string][]diff.Option{
					"default":          nil,
					"optimal":          {diff.Optimal()},
					"fast":             {diff.Fast()},
					"indent-heuristic": {textdiff.IndentHeuristic()},
				}

				lines := func(s string) int {
					n := strings.Count(s, "\n")
					if len(s) > 0 && s[len(s)-1] != '\n' {
						n++
					}
					return n
				}
				old := change.old
				new := change.new
				for len(old) > 0 && len(new) > 0 && old[0] == new[0] {
					old = old[1:]
					new = new[1:]
				}
				for len(old) > 0 && len(new) > 0 && old[len(old)-1] == new[len(new)-1] {
					old = old[:len(old)-1]
					new = new[:len(new)-1]
				}
				N, M := lines(old), lines(new)

				for variant, opts := range variants {
					if results != nil {
						start := time.Now()
						hunks := textdiff.Hunks(change.old, change.new, opts...)
						duration := time.Since(start)
						nedits := 0
						for _, hunk := range hunks {
							for _, edits := range hunk.Edits {
								if edits.Op == diff.Delete || edits.Op == diff.Insert {
									nedits++
								}
							}
						}
						results <- result{
							commitID: change.commitID,
							file:     change.filename,
							variant:  variant,
							N:        N,
							M:        M,
							D:        nedits,
							duration: duration,
						}
					}

					if cfg.validate {
						unified := textdiff.Unified(change.old, change.new, opts...)
						patched, err := unixpatch.Patch(change.old, unified)
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
					}
				}
				processed.Add(1)
			}
		}()
	}

	// Render progress
	var ioWG sync.WaitGroup
	render := func() {
		const width = 60
		commits := commitsDone.Load()
		processed := processed.Load()
		progress := float64(commits) / float64(len(commitIDs))
		whole := int(progress * width)
		remainder := math.Mod(progress*width, 1)
		last := bars[max(0, min(len(bars), int(remainder*float64(len(bars)))))]
		if width-whole < 1 {
			last = ""
		}
		bar := strings.Repeat(bars[len(bars)-1], whole) + last
		var commitsPerSec, procPerSec int
		if commits > 0 {
			commitsPerSec = int((time.Duration(commits) * time.Second) / time.Since(start))
		}
		if processed > 0 {
			procPerSec = int((time.Duration(processed) * time.Second) / time.Since(start))
		}
		fmt.Printf("\r[%-*s] % 3.1f%% (%d commits/s, %d evals/s) ", width, bar, 100*progress, commitsPerSec, procPerSec)
	}
	ioWG.Add(1)
	go func() {
		defer ioWG.Done()
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
	if cfg.stats != "" {
		go func() {
			defer ioWG.Done()
			w := bufio.NewWriter(stats)
			w.WriteString("commit_id,file,variant,N,M,D,duration_ns\n")
			for result := range results {
				_, err := fmt.Fprintf(w, "%s,%s,%s,%d,%d,%d,%d\n", result.commitID, result.file, result.variant, result.N, result.M, result.D, result.duration.Nanoseconds())
				if err != nil {
					notes <- note{
						prefix: result.commitID + ":" + result.file,
						msg:    fmt.Sprintf("failed to write stats: %v", err),
					}
				}
			}
			err := w.Flush()
			if err != nil {
				notes <- note{
					prefix: "",
					msg:    fmt.Sprintf("failed to flush stats: %v", err),
				}
			}
		}()
	}

	// Shutdown
	changesWG.Wait()
	git.Close()
	close(changes)
	processWG.Wait()
	close(done)
	if results != nil {
		close(results)
	}
	ioWG.Wait()

	return nil
}
