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

// gitdiff is a tool that can be used with git using GIT_EXTERNAL_DIFF.
//
// This is not generally useful and it has some weird defaults. The use case for it is to work with
// the slider evaluation in https://github.com/mhagger/diff-slider-tools. The evaluation can be used
// with a small local modification of the repositories run-comparison script:
//
// Adding this snippet
//
//	git_znkr() {
//	   GIT_EXTERNAL_DIFF=${HOME}/Projects/diff/gitdiff git -C corpus/$1.git $GIT_OPTS diff "$2" "$3" --
//	}
//
// allows us to compare against git's implementation of indent heuristics. The comparison is not
// 100% because we sometimes return different diffs than git, but overall the quality of the
// result is about the same.
package main

import (
	"fmt"
	"os"

	"znkr.io/diff"
	"znkr.io/diff/textdiff"
)

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
}

func run(args []string) error {
	if len(args) < 8 {
		return fmt.Errorf("expected at least 8 args, got %v: %v", len(args), args)
	}

	path, oldFile, oldHex, oldMode, newFile, newHex, newMode := args[1], args[2], args[3], args[4], args[5], args[6], args[7]
	_, _, _, _, _, _, _ = path, oldFile, oldHex, oldMode, newFile, newHex, newMode

	var old []byte
	if oldFile != "/dev/null" {
		var err error
		old, err = os.ReadFile(oldFile)
		if err != nil {
			return fmt.Errorf("reading old file: %v", err)
		}
	}

	var new []byte
	if newFile != "/dev/null" {
		var err error
		new, err = os.ReadFile(newFile)
		if err != nil {
			return fmt.Errorf("reading new file: %v", err)
		}
	}

	diff := textdiff.Unified(old, new, textdiff.IndentHeuristic(), diff.Context(20))

	fmt.Printf("diff --git a/%s b/%s\n", path, path)
	fmt.Printf("index %s..%s %s\n", oldHex[:10], newHex[:10], newMode)
	fmt.Printf("--- a/%s\n", path)
	fmt.Printf("+++ b/%s\n", path)
	os.Stdout.Write(diff)

	return nil
}
