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

// Parsecorpus writes the unified diffs that git diff, GNU diff and BSD diff
// write for each case below to a txtar archive named after the case, in the
// corpus of textdiff.ParseUnified. Archives whose names start with "manual_" are
// written by hand and left alone.
//
// Each archive holds the sections x and y with the inputs, diff with the tool
// output, and either header with the two labels as Go strings or error with
// the error of ParseUnified. A section name ending in "-noeol" has no newline
// at the end of its content.
//
// Usage, from the module root:
//
//	go run ./internal/cmd/parsecorpus -gnudiff=/path/to/gnu/diff
//
// With nix, GNU diff is $(nix shell nixpkgs#diffutils -c which diff).
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/tools/txtar"
)

var (
	gnuDiff = flag.String("gnudiff", "", "path of GNU diff")
	bsdDiff = flag.String("bsddiff", "/usr/bin/diff", "path of BSD diff")
	gitTool = flag.String("git", "git", "path of git")
	outDir  = flag.String("out", "textdiff/testdata/parse", "output directory")
)

// corpusCase is a diff written by one tool.
type corpusCase struct {
	name    string
	comment string
	tool    string   // "git", "git-format-patch", "git-no-index", "gnu" or "bsd".
	args    []string // Arguments after "diff" or "format-patch" and the default arguments.
	config  []string // git configuration as "key=value".
	x, y    string
	// File names of x and y. An empty name in a git case means the file does
	// not exist. The default is "x" and "y", or "f" for both in a git or
	// git-format-patch case.
	xname, yname string
	yexec        bool                                // Makes y executable in a git case.
	setup        func(dir string, second bool) error // Runs before each git commit.
	// Labels that ParseUnified returns. The default is the file names, with
	// the prefixes a/ and b/ in a git case.
	labels   []string
	noHeader bool   // Reports whether the diff has no file header.
	err      string // Error that ParseUnified returns.
}

const (
	text    = "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n"
	changed = "one\nTWO\nthree\nfour\nfive\nsix\nseven\neight\nNINE\nten\n"
	code    = "int f()\n{\n\t1;\n\t2;\n\t3;\n\t4;\n\t5;\n}\n"
	code2   = "int f()\n{\n\t1;\n\t2;\n\t3;\n\t4;\n\tX;\n}\n"
	binary  = "\x00\x01\x02"
	binary2 = "\x00\x01\x03"
	// Lines that look like patch syntax after a line prefix.
	lookalike  = "-- a\n++ b\n@@ -1 +1 @@\n\\ c\ndiff --git a b\n"
	lookalike2 = "x\n" + lookalike + "y\n"
)

func cases() []corpusCase {
	shared := []corpusCase{
		{name: "basic", comment: "Two changes in one hunk, with timestamps after the labels.", x: text, y: changed},
		{name: "hunks", comment: "Two hunks.", args: []string{"-U1"}, x: text, y: changed},
		{name: "context_0", comment: "No context, so the insertion has an empty range.", args: []string{"-U0"}, x: text, y: strings.Replace(text, "five\n", "five\nnew\n", 1)},
		{name: "context_large", comment: "A context larger than the input.", args: []string{"-U100"}, x: text, y: changed},
		{name: "function_name", comment: "Text after the hunk header.", args: []string{"-p"}, x: code, y: code2},
		{name: "no_newline_x", comment: "x has no newline at the end.", x: "a\nb", y: "a\nb\n"},
		{name: "no_newline_y", comment: "y has no newline at the end.", x: "a\nb\n", y: "a\nb"},
		{name: "no_newline_both", comment: "Neither input has a newline at the end, and the last line is context.", x: "a\nb", y: "c\nb"},
		{name: "no_newline_both_changed", comment: "Neither input has a newline at the end, and the last line changes.", x: "a\nb", y: "a\nc"},
		{name: "x_empty", comment: "x is empty.", x: "", y: "a\nb\n"},
		{name: "y_empty", comment: "y is empty.", x: "a\nb\n", y: ""},
		{name: "identical", comment: "Equal inputs have an empty diff.", x: text, y: text, noHeader: true},
		{name: "crlf", comment: "Lines end in CRLF.", x: "a\r\nb\r\n", y: "a\r\nc\r\n"},
		{name: "empty_lines", comment: "Empty lines as context.", x: "a\n\nb\n\n", y: "a\n\nc\n\n"},
		{name: "lookalike", comment: "Body lines that look like patch syntax without their prefix.", x: lookalike2, y: "x\ny\n" + lookalike},
		{name: "space_name", comment: "File names with a space.", x: "a\n", y: "b\n", xname: "x x", yname: "y y"},
		{name: "binary", comment: "Binary files.", x: binary, y: binary2, err: "line 1: binary diff"},
	}
	var cs []corpusCase
	for _, tool := range []string{"gnu", "bsd"} {
		for _, c := range shared {
			c.tool = tool
			c.name = tool + "_" + c.name
			cs = append(cs, c)
		}
	}
	cs = append(cs,
		corpusCase{name: "gnu_quoted_names", tool: "gnu", comment: "GNU diff quotes file names with special characters.", x: "a\n", y: "b\n", xname: "x\tx", yname: "y\n\"é\\"},
		corpusCase{name: "gnu_label", tool: "gnu", comment: "Labels set with --label, written without a timestamp.", args: []string{"--label", "old file", "--label", "new"}, x: "a\n", y: "b\n", labels: []string{"old file", "new"}},
		corpusCase{name: "gnu_suppress_blank_empty", tool: "gnu", comment: "Empty context lines are written without a prefix.", args: []string{"--suppress-blank-empty"}, x: "a\n\nb\n", y: "a\n\nc\n"},
		corpusCase{name: "gnu_color", tool: "gnu", comment: "Colored output.", args: []string{"--color=always"}, x: text, y: changed, err: "line 1: ANSI escape sequence"},
		corpusCase{name: "bsd_quote_name", tool: "bsd", comment: "BSD diff writes file names that start with a quote unquoted.", x: "a\n", y: "b\n", xname: "\"x\".txt", yname: "\"y\" y"},
		corpusCase{name: "bsd_newline_name", tool: "bsd", comment: "BSD diff writes a file name with a newline unquoted, which splits the header.", x: "a\n", y: "b\n", xname: "x", yname: "y\ny", err: "line 3: missing hunk header after file header"},

		corpusCase{name: "git_basic", comment: "Two changes in one hunk.", x: text, y: changed},
		corpusCase{name: "git_hunks", comment: "Two hunks.", args: []string{"-U1"}, x: text, y: changed},
		corpusCase{name: "git_context_0", comment: "No context, so the insertion has an empty range.", args: []string{"-U0"}, x: text, y: strings.Replace(text, "five\n", "five\nnew\n", 1)},
		corpusCase{name: "git_function_name", comment: "Text after the hunk header.", x: code, y: code2},
		corpusCase{name: "git_no_newline_x", comment: "x has no newline at the end.", x: "a\nb", y: "a\nb\n"},
		corpusCase{name: "git_no_newline_y", comment: "y has no newline at the end.", x: "a\nb\n", y: "a\nb"},
		corpusCase{name: "git_no_newline_both", comment: "Neither input has a newline at the end, and the last line is context.", x: "a\nb", y: "c\nb"},
		corpusCase{name: "git_crlf", comment: "Lines end in CRLF.", x: "a\r\nb\r\n", y: "a\r\nc\r\n"},
		corpusCase{name: "git_lookalike", comment: "Body lines that look like patch syntax without their prefix.", x: lookalike2, y: "x\ny\n" + lookalike},
		corpusCase{name: "git_new_file", comment: "A new file is diffed against /dev/null.", y: "a\nb\n", xname: "", yname: "f"},
		corpusCase{name: "git_deleted_file", comment: "A deleted file is diffed against /dev/null.", x: "a\nb\n", xname: "f", yname: ""},
		corpusCase{name: "git_new_empty_file", comment: "A new empty file has no file header and no hunks.", xname: "", yname: "f", err: "line 3: git diff without a content change"},
		corpusCase{name: "git_deleted_empty_file", comment: "A deleted empty file has no file header and no hunks.", xname: "f", yname: "", err: "line 3: git diff without a content change"},
		corpusCase{name: "git_mode", comment: "A mode change without a content change has no file header and no hunks.", x: text, y: text, yexec: true, err: "line 3: git diff without a content change"},
		corpusCase{name: "git_mode_and_content", comment: "A mode change with a content change.", x: text, y: changed, yexec: true},
		corpusCase{name: "git_rename", comment: "A rename without a content change has no file header and no hunks.", args: []string{"-M"}, x: text, y: text, xname: "f", yname: "g", err: "line 4: git diff without a content change"},
		corpusCase{name: "git_rename_and_content", comment: "A rename with a content change.", args: []string{"-M"}, x: text, y: strings.Replace(text, "ten\n", "TEN\n", 1), xname: "f", yname: "g"},
		corpusCase{name: "git_space_name", comment: "git writes a tab after a file name with a space.", x: "a\n", y: "b\n", xname: "f f", yname: "f f"},
		corpusCase{name: "git_quote_path", comment: "With core.quotePath, git quotes non-ASCII file names with octal escapes.", config: []string{"core.quotePath=true"}, x: "a\n", y: "b\n", xname: "é f", yname: "é f"},
		corpusCase{name: "git_quoted_name", comment: "git quotes file names with special characters.", x: "a\n", y: "b\n", xname: "f\t\"\\", yname: "f\t\"\\"},
		corpusCase{name: "git_no_prefix", comment: "Labels without a/ and b/.", args: []string{"--no-prefix"}, x: "a\n", y: "b\n", labels: []string{"f", "f"}},
		corpusCase{name: "git_suppress_blank_empty", comment: "Empty context lines are written without a prefix.", config: []string{"diff.suppressBlankEmpty=true"}, x: "a\n\nb\n", y: "a\n\nc\n"},
		corpusCase{name: "git_binary", comment: "Binary files.", x: binary, y: binary2, err: "line 3: binary diff"},
		corpusCase{name: "git_binary_patch", comment: "Binary files with --binary.", args: []string{"--binary"}, x: binary, y: binary2, err: "line 3: binary diff"},
		corpusCase{name: "git_color", comment: "Colored output.", args: []string{"--color=always"}, x: text, y: changed, err: "line 1: ANSI escape sequence"},
		corpusCase{
			name: "git_two_files", comment: "A diff of two files.", x: text, y: changed,
			setup: func(dir string, second bool) error {
				return appendFile(filepath.Join(dir, "g"), "line\n")
			},
			err: "line 18: patch contains more than one file",
		},
		corpusCase{
			name: "git_two_files_no_hunks", comment: "A diff of two files, where the first has no hunks.", x: text, y: text, xname: "a", yname: "a",
			setup: func(dir string, second bool) error {
				if !second {
					if err := os.Chmod(filepath.Join(dir, "a"), 0o755); err != nil {
						return err
					}
				}
				return appendFile(filepath.Join(dir, "b"), "line\n")
			},
			err: "line 4: patch contains more than one file",
		},
		corpusCase{name: "git_format_patch", tool: "git-format-patch", comment: "git format-patch output, with a commit message before the diff and a signature after it.", args: []string{"-1"}, x: text, y: changed},
		corpusCase{name: "git_format_patch_two", tool: "git-format-patch", comment: "git format-patch output for two commits.", args: []string{"--root"}, x: text, y: changed, err: "line 31: patch contains more than one file"},
		corpusCase{name: "git_no_index", tool: "git-no-index", comment: "git diff --no-index of two files outside a repository.", x: text, y: changed},
	)
	for i := range cs {
		c := &cs[i]
		if c.tool == "" {
			c.tool = "git"
		}
		git := c.tool == "git" || c.tool == "git-format-patch"
		if git && c.xname == "" && c.yname == "" && c.x != "" {
			c.xname, c.yname = "f", "f"
		}
		if !git && c.xname == "" {
			c.xname, c.yname = "x", "y"
		}
		if c.labels != nil || c.noHeader || c.err != "" {
			continue
		}
		switch c.tool {
		case "git", "git-format-patch":
			c.labels = []string{"/dev/null", "/dev/null"}
			if c.xname != "" {
				c.labels[0] = "a/" + c.xname
			}
			if c.yname != "" {
				c.labels[1] = "b/" + c.yname
			}
		case "git-no-index":
			c.labels = []string{"a/" + c.xname, "b/" + c.yname}
		default:
			c.labels = []string{c.xname, c.yname}
		}
	}
	return cs
}

// appendFile appends s to the file name, creating it if needed.
func appendFile(name, s string) error {
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(s); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func main() {
	log.SetFlags(0)
	flag.Parse()
	if *gnuDiff == "" {
		log.Fatal("-gnudiff is required")
	}
	gitVersion := firstLine(*gitTool, "--version")
	versions := map[string]string{
		"gnu":              firstLine(*gnuDiff, "--version"),
		"bsd":              firstLine(*bsdDiff, "--version"),
		"git":              gitVersion,
		"git-no-index":     gitVersion,
		"git-format-patch": gitVersion,
	}
	if !strings.Contains(versions["gnu"], "GNU") {
		log.Fatalf("%s is not GNU diff: %s", *gnuDiff, versions["gnu"])
	}
	if !strings.Contains(versions["bsd"], "BSD") {
		log.Fatalf("%s is not BSD diff: %s", *bsdDiff, versions["bsd"])
	}
	for _, c := range cases() {
		cmd, out, err := run(c)
		if err != nil {
			log.Fatalf("%s: %v", c.name, err)
		}
		ar := &txtar.Archive{
			Comment: fmt.Appendf(nil, "%s\n\nCommand: %s\nTool: %s\n", c.comment, cmd, versions[c.tool]),
		}
		ar.Files = append(ar.Files, section("x", c.x), section("y", c.y), section("diff", string(out)))
		if c.labels != nil {
			ar.Files = append(ar.Files, txtar.File{Name: "header", Data: fmt.Appendf(nil, "%s\n%s\n", strconv.Quote(c.labels[0]), strconv.Quote(c.labels[1]))})
		}
		if c.err != "" {
			ar.Files = append(ar.Files, txtar.File{Name: "error", Data: []byte(c.err + "\n")})
		}
		if err := os.WriteFile(filepath.Join(*outDir, c.name+".test"), txtar.Format(ar), 0o644); err != nil {
			log.Fatal(err)
		}
	}
}

// section returns the archive file name with content s.
func section(name, s string) txtar.File {
	if s != "" && !strings.HasSuffix(s, "\n") {
		return txtar.File{Name: name + "-noeol", Data: []byte(s + "\n")}
	}
	return txtar.File{Name: name, Data: []byte(s)}
}

// firstLine returns the first line of the output of the command.
func firstLine(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		log.Fatalf("%s: %v", name, err)
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return line
}

// run runs the tool of c and returns the command line and its output.
func run(c corpusCase) (string, []byte, error) {
	dir, err := os.MkdirTemp("", "corpus-*")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(dir)

	switch c.tool {
	case "gnu", "bsd", "git-no-index":
		// Fixed times make the timestamps in the labels reproducible.
		t := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		if err := writeFile(filepath.Join(dir, c.xname), c.x, false, t); err != nil {
			return "", nil, err
		}
		if err := writeFile(filepath.Join(dir, c.yname), c.y, false, t.Add(time.Second)); err != nil {
			return "", nil, err
		}
		name, tool, args := "diff", *gnuDiff, append([]string{"-u"}, c.args...)
		switch c.tool {
		case "bsd":
			tool = *bsdDiff
		case "git-no-index":
			name, tool, args = "git", *gitTool, append([]string{"diff", "--no-index"}, c.args...)
		}
		args = append(args, c.xname, c.yname)
		out, err := command(dir, true, tool, args...)
		return name + " " + quoteArgs(args), out, err
	case "git", "git-format-patch":
		var config []string
		for _, kv := range c.config {
			config = append(config, "-c", kv)
		}
		git := func(args ...string) ([]byte, error) {
			return command(dir, false, *gitTool, append(slices.Clone(config), args...)...)
		}
		if _, err := git("init", "-q"); err != nil {
			return "", nil, err
		}
		if c.xname != "" {
			if err := writeFile(filepath.Join(dir, c.xname), c.x, false, time.Time{}); err != nil {
				return "", nil, err
			}
		}
		if c.setup != nil {
			if err := c.setup(dir, false); err != nil {
				return "", nil, err
			}
		}
		if _, err := git("add", "-A"); err != nil {
			return "", nil, err
		}
		commit := func(msg string) error {
			_, err := git("-c", "user.name=corpus", "-c", "user.email=corpus@example.com", "commit", "-q", "--allow-empty", "-m", msg)
			return err
		}
		if err := commit("x"); err != nil {
			return "", nil, err
		}
		if c.xname != "" && c.xname != c.yname {
			if err := os.Remove(filepath.Join(dir, c.xname)); err != nil {
				return "", nil, err
			}
		}
		if c.yname != "" {
			if err := writeFile(filepath.Join(dir, c.yname), c.y, c.yexec, time.Time{}); err != nil {
				return "", nil, err
			}
		}
		if c.setup != nil {
			if err := c.setup(dir, true); err != nil {
				return "", nil, err
			}
		}
		if _, err := git("add", "-A"); err != nil {
			return "", nil, err
		}
		args := append([]string{"diff", "--cached"}, c.args...)
		if c.tool == "git-format-patch" {
			if err := commit("y"); err != nil {
				return "", nil, err
			}
			args = append([]string{"format-patch", "--stdout"}, c.args...)
		}
		out, err := git(args...)
		return "git " + quoteArgs(append(config, args...)), out, err
	}
	return "", nil, fmt.Errorf("unknown tool %q", c.tool)
}

// writeFile writes s to the file name. If exec is set, the file is executable.
// If t is not zero, it is the modification time of the file.
func writeFile(name, s string, exec bool, t time.Time) error {
	mode := os.FileMode(0o644)
	if exec {
		mode = 0o755
	}
	if err := os.WriteFile(name, []byte(s), mode); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	if t.IsZero() {
		return nil
	}
	return os.Chtimes(name, t, t)
}

// command runs the tool name with args in dir and returns its standard output.
// If differ is set, the tool is a diff, and exit status 1, which means the
// inputs differ, is not an error.
func command(dir string, differ bool, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	// The environment keeps the user's configuration out of the output, and
	// fixed dates make commits reproducible.
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "TZ=UTC", "LC_ALL=C", "HOME="+dir, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_DATE=2026-01-02T03:04:05Z", "GIT_COMMITTER_DATE=2026-01-02T03:04:05Z")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if exitErr := (*exec.ExitError)(nil); differ && errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s %s: %v\n%s", name, quoteArgs(args), err, stderr.Bytes())
	}
	return out, nil
}

// quoteArgs returns args as a shell command line.
func quoteArgs(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\n\"'\\$|&;<>()*?[]#~`") || a != strings.ToValidUTF8(a, "") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		q[i] = a
	}
	return strings.Join(q, " ")
}
