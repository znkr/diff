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

package main

import (
	"errors"
	"flag"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"

	"znkr.io/diff/internal/unixtools"
)

var validate = flag.Bool("validate", false, "compare the output with git")

func TestMain(m *testing.M) {
	// git runs the test binary as GIT_EXTERNAL_DIFF with this variable set.
	if os.Getenv("GITDIFF_RUN_MAIN") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestMatchesGit checks that gitdiff writes the same output as git for each
// kind of change.
func TestMatchesGit(t *testing.T) {
	if !*validate {
		t.Skip("requires -validate")
	}
	env := externalDiffEnv(t)
	lines := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	edited := "1\n2\n3\n4\nfive\n6\n7\n8\n9\n10\n"
	type files = map[string]unixtools.File
	tests := []struct {
		name          string
		before, after files
	}{
		{"modify", files{"f.txt": {Content: lines}}, files{"f.txt": {Content: edited}}},
		{"new", files{}, files{"f.txt": {Content: lines}}},
		{"delete", files{"f.txt": {Content: lines}}, files{}},
		{"mode", files{"f.sh": {Content: lines}}, files{"f.sh": {Content: lines, Executable: true}}},
		{"mode-and-modify", files{"f.sh": {Content: lines}}, files{"f.sh": {Content: edited, Executable: true}}},
		{"rename", files{"a.txt": {Content: lines}}, files{"b.txt": {Content: lines}}},
		{"rename-and-modify", files{"a.txt": {Content: lines}}, files{"b.txt": {Content: edited}}},
		{"rename-and-mode", files{"a.sh": {Content: lines}}, files{"b.sh": {Content: edited, Executable: true}}},
		{"subdirectory", files{"d/f.txt": {Content: lines}}, files{"d/f.txt": {Content: edited}}},
		{"space", files{"f g.txt": {Content: lines}}, files{"f g.txt": {Content: edited}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, err := unixtools.GitDiff(tt.before, tt.after)
			if err != nil {
				t.Fatal(err)
			}
			got, err := unixtools.GitDiff(tt.before, tt.after, env...)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("gitdiff wrote:\n%s\ngit wrote:\n%s", got, want)
			}
		})
	}
}

// TestHeaderQuoting checks that gitdiff writes the "---" and "+++" lines as
// git does with core.quotePath=false.
func TestHeaderQuoting(t *testing.T) {
	if !*validate {
		t.Skip("requires -validate")
	}
	env := externalDiffEnv(t)
	names := []string{
		"plain.txt",
		"with space.txt",
		`quo"te.txt`,
		`"leading-quote.txt`,
		`back\slash.txt`,
		"nön-ascii.txt",
		"x'y$z*?[.txt",
		"tab\t.txt",
		"new\nline.txt",
		"space and\nnewline.txt",
		"bell\a.txt",
		"backspace\b.txt",
		"vtab\v.txt",
		"formfeed\f.txt",
		"return\r.txt",
		"esc\x1b.txt",
		"soh\x01.txt",
		"del\x7f.txt",
		"lone\x80byte.txt",
	}
	for _, name := range names {
		t.Run(strconv.Quote(name), func(t *testing.T) {
			before := map[string]unixtools.File{name: {Content: "a\n"}}
			after := map[string]unixtools.File{name: {Content: "b\n"}}
			want, err := unixtools.GitDiff(before, after)
			var pathErr *fs.PathError
			if errors.As(err, &pathErr) {
				t.Skipf("file system rejects the name: %v", err)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := unixtools.GitDiff(before, after, env...)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := headerLines(got), headerLines(want); got != want {
				t.Errorf("gitdiff wrote header %q, git wrote %q", got, want)
			}
		})
	}
}

// externalDiffEnv returns the environment variables that make git run the test
// binary as gitdiff.
func externalDiffEnv(t *testing.T) []string {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// git runs GIT_EXTERNAL_DIFF with the shell.
	quoted := "'" + strings.ReplaceAll(self, "'", `'\''`) + "'"
	return []string{"GIT_EXTERNAL_DIFF=" + quoted, "GITDIFF_RUN_MAIN=1"}
}

// headerLines returns the "---" and "+++" lines of a diff.
func headerLines(diff string) string {
	var b strings.Builder
	for line := range strings.Lines(diff) {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			b.WriteString(line)
		}
	}
	return b.String()
}
