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

// eval validates the diffing algorithm and the unified diff parser on the
// history of a git repository. For each changed file, it checks that the unix
// patch tool turns the old file into the new one with the diff of Unified,
// that ParseUnified returns the hunks of that diff, and that the hunks of
// git's diff, read by ParseUnified, turn the old file into the new one.
//
// With -bench, eval times Hunks on each changed file instead, one file at a
// time, and writes the timings to a CSV file.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"znkr.io/diff"
	"znkr.io/diff/internal/cmd/eval/internal/git"
	"znkr.io/diff/internal/unixtools"
	"znkr.io/diff/textdiff"
)

type config struct {
	repo     string
	sample   int
	parallel int
	bench    string
}

func main() {
	var cfg config
	flag.StringVar(&cfg.repo, "repo", "", "repository to use for evaluation")
	flag.IntVar(&cfg.sample, "sample", 0, "if >0, sample commits to the value of the flag")
	flag.IntVar(&cfg.parallel, "parallel", runtime.GOMAXPROCS(0), "number of files to validate in parallel")
	flag.StringVar(&cfg.bench, "bench", "", "time the diffs instead of validating them and write the timings to this CSV `file`")
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
	if cfg.bench != "" {
		var err error
		stats, err = os.Create(cfg.bench)
		if err != nil {
			return fmt.Errorf("creating stats file: %v", err)
		}
		defer stats.Close()
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
		commitID   string
		filename   string
		old, new   string
		gitPatches [][]textdiff.Hunk[string] // Hunks of git's diffs of the file, in the order they apply.
	}
	changes := make(chan change)
	var changesWG sync.WaitGroup
	chunkSize := max(1, len(commitIDs)/(4*runtime.GOMAXPROCS(0)))
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
							commitID:   commitID,
							filename:   file.Name,
							old:        res[0],
							new:        res[1],
							gitPatches: file.Patches,
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
	workers := cfg.parallel
	if cfg.bench != "" {
		results = make(chan result)
		// Diffs that run at the same time slow each other down.
		workers = 1
	}
	validate := cfg.bench == ""
	for range workers {
		processWG.Add(1)
		go func() {
			defer processWG.Done()
			for change := range changes {
				if validate {
					if got, err := applyPatches(change.old, change.gitPatches); err != nil || got != change.new {
						notes <- note{
							prefix: change.commitID + ":" + change.filename,
							msg:    fmt.Sprintf("hunks of git's diff don't turn the old file into the new one: %v", err),
						}
					}
				}
				variants := []struct {
					name string
					opts []diff.Option
				}{
					{"default", nil},
					{"minimal", []diff.Option{diff.Minimal()}},
					{"fast", []diff.Option{diff.Fast()}},
					{"indent-heuristic", []diff.Option{textdiff.IndentHeuristic()}},
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

				// Each round times every variant once, and each variant keeps
				// its fastest time. A single timing often includes a garbage
				// collection, a pause of the goroutine, or a cold cache.
				durations := make([]time.Duration, len(variants))
				hunks := make([][]textdiff.Hunk[string], len(variants))
				if results != nil {
					start := time.Now()
					for round := 0; round < 5 && (round < 2 || time.Since(start) < 100*time.Millisecond); round++ {
						for i, v := range variants {
							start := time.Now()
							hunks[i] = textdiff.Hunks(change.old, change.new, v.opts...)
							if d := time.Since(start); round == 0 || d < durations[i] {
								durations[i] = d
							}
						}
					}
				}
				for i, v := range variants {
					variant, opts := v.name, v.opts
					if results != nil {
						nedits := 0
						for _, hunk := range hunks[i] {
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
							duration: durations[i],
						}
					}

					if validate {
						unified := textdiff.Unified(change.old, change.new, opts...)
						patched, err := unixtools.Patch(change.old, unified)
						if err != nil {
							notes <- note{
								prefix: change.commitID + ":" + change.filename,
								msg:    fmt.Sprintf("failed to run patch: %v", err),
							}
						}
						if change.new != patched {
							notes <- note{
								prefix: change.commitID + ":" + change.filename,
								msg:    fmt.Sprintf("file is different after applying patch. got:\n%s\nwant:\n%s", patched, change.new),
							}
						}
						if err := checkParse(unified, textdiff.Hunks(change.old, change.new, opts...)); err != nil {
							notes <- note{
								prefix: change.commitID + ":" + change.filename,
								msg:    err.Error(),
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
	var statsErr error
	if cfg.bench != "" {
		ioWG.Add(1)
		go func() {
			defer ioWG.Done()
			// w keeps the first write error and returns it from Flush.
			w := bufio.NewWriter(stats)
			w.WriteString("commit_id,file,variant,N,M,D,duration_ns\n")
			for result := range results {
				fmt.Fprintf(w, "%s,%s,%s,%d,%d,%d,%d\n", result.commitID, result.file, result.variant, result.N, result.M, result.D, result.duration.Nanoseconds())
			}
			statsErr = w.Flush()
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

	if statsErr != nil {
		return fmt.Errorf("writing stats file: %v", statsErr)
	}
	if stats != nil {
		if err := stats.Close(); err != nil {
			return fmt.Errorf("closing stats file: %v", err)
		}
	}
	return nil
}

// checkParse returns an error if ParseUnified doesn't return hunks for
// unified, the diff that Unified writes for hunks.
func checkParse(unified string, hunks []textdiff.Hunk[string]) error {
	files, err := textdiff.ParseUnified(unified, textdiff.ParseHeaderless())
	if err != nil {
		return fmt.Errorf("parsing unified diff: %v", err)
	}
	var got []textdiff.Hunk[string]
	if len(files) == 1 {
		got = files[0].Hunks
	}
	if len(got) != len(hunks) || len(got) > 0 && !reflect.DeepEqual(got, hunks) {
		return fmt.Errorf("parsed hunks differ from Hunks")
	}
	return nil
}

// applyPatches returns x with each patch applied in order.
func applyPatches(x string, patches [][]textdiff.Hunk[string]) (string, error) {
	for _, hunks := range patches {
		var err error
		if x, err = applyHunks(x, hunks); err != nil {
			return "", err
		}
	}
	return x, nil
}

// applyHunks returns x with hunks applied at their exact positions.
func applyHunks(x string, hunks []textdiff.Hunk[string]) (string, error) {
	xl := slices.Collect(strings.Lines(x))
	var b strings.Builder
	pos := 0
	for _, h := range hunks {
		if h.EndLineNoX > len(xl) {
			return "", fmt.Errorf("hunk ends at line %d after the end of the file", h.EndLineNoX)
		}
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
