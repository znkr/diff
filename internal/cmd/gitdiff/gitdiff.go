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
// This is not generally useful and it has some weird defaults. The use case for
// it is to work with the slider evaluation in
// https://github.com/mhagger/diff-slider-tools. The evaluation can be used with
// a small local modification of the repositories run-comparison script:
//
// Adding this snippet
//
//	git_znkr() {
//	   GIT_EXTERNAL_DIFF=${HOME}/Projects/diff/gitdiff \
//	     git -C corpus/$1.git $GIT_OPTS diff "$2" "$3" --
//	}
//
// allows us to compare against git's implementation of indent heuristics. The
// comparison is not 100% because we sometimes return different diffs than git,
// but overall the quality of the result is about the same.
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
		os.Exit(1)
	}
}

func run(args []string) error {
	// For an unmerged path, git passes only the path.
	if len(args) == 2 {
		fmt.Printf("* Unmerged path %s\n", args[1])
		return nil
	}
	if len(args) < 8 {
		return fmt.Errorf("expected at least 8 args, got %v: %v", len(args), args)
	}

	path, oldFile, oldHex, oldMode, newFile, newHex, newMode := args[1], args[2], args[3], args[4], args[5], args[6], args[7]
	// For a rename or copy, git also passes the new path and the extended
	// header lines, from "similarity index" to "index".
	newPath, extHeader := path, ""
	if len(args) >= 10 {
		newPath, extHeader = args[8], args[9]
	}

	var old []byte
	oldLabel := "/dev/null"
	if oldFile != "/dev/null" {
		oldLabel = "a/" + path
		var err error
		old, err = os.ReadFile(oldFile)
		if err != nil {
			return fmt.Errorf("reading old file: %v", err)
		}
	}

	var new []byte
	newLabel := "/dev/null"
	if newFile != "/dev/null" {
		newLabel = "b/" + newPath
		var err error
		new, err = os.ReadFile(newFile)
		if err != nil {
			return fmt.Errorf("reading new file: %v", err)
		}
	}

	// Unlike git, this line has no quoting. For a path with a control
	// character, '"' or '\\' it differs from git's, and a newline splits it.
	fmt.Printf("diff --git a/%s b/%s\n", path, newPath)
	switch {
	case oldFile == "/dev/null":
		fmt.Printf("new file mode %s\n", newMode)
		fmt.Printf("index %s..%s\n", abbrev(oldHex), abbrev(newHex))
	case newFile == "/dev/null":
		fmt.Printf("deleted file mode %s\n", oldMode)
		fmt.Printf("index %s..%s\n", abbrev(oldHex), abbrev(newHex))
	case extHeader != "":
		// Like git, write the mode change before the rename or copy.
		if oldMode != newMode {
			fmt.Printf("old mode %s\nnew mode %s\n", oldMode, newMode)
		}
		fmt.Print(extHeader)
	case oldMode != newMode:
		fmt.Printf("old mode %s\nnew mode %s\n", oldMode, newMode)
		if oldHex != newHex {
			fmt.Printf("index %s..%s\n", abbrev(oldHex), abbrev(newHex))
		}
	default:
		fmt.Printf("index %s..%s %s\n", abbrev(oldHex), abbrev(newHex), newMode)
	}
	return textdiff.WriteUnified(os.Stdout, old, new,
		textdiff.IndentHeuristic(),
		diff.Context(20),
		textdiff.UnifiedHeader(oldLabel, newLabel),
	)
}

// abbrev returns the first 10 digits of the hex hash, or 10 zeros for ".",
// which git passes for a missing file.
func abbrev(hash string) string {
	if hash == "." {
		return "0000000000"
	}
	return hash[:10]
}
