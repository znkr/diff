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

package unixtools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitApply applies diff with "git apply" to a file named name with the content
// orig, and returns the new content of the file. git apply finds the file from
// the header of diff.
func GitApply(name, orig, diff string) (string, error) {
	return applyToFile(name, orig, diff, func(dir string) *exec.Cmd {
		return gitCommand(dir, nil, "apply", "patch.diff")
	})
}

// File is the content and mode of a file for [GitDiff].
type File struct {
	Content    string
	Executable bool
}

// GitDiff commits the files in before to a new repository, stages the files in
// after in their place, and returns the output of "git diff --cached -M -U20".
// The map keys are file names. git runs with core.quotePath=false,
// core.abbrev=10 and the extra environment variables env, such as
// GIT_EXTERNAL_DIFF.
func GitDiff(before, after map[string]File, env ...string) (string, error) {
	dir, err := os.MkdirTemp("", "gitdiff-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	identity := []string{
		"GIT_AUTHOR_NAME=unixtools", "GIT_AUTHOR_EMAIL=unixtools@example.com",
		"GIT_COMMITTER_NAME=unixtools", "GIT_COMMITTER_EMAIL=unixtools@example.com",
	}
	if err := runGit(dir, nil, "init", "-q"); err != nil {
		return "", err
	}
	if err := writeFiles(dir, before); err != nil {
		return "", err
	}
	if err := runGit(dir, nil, "add", "-A"); err != nil {
		return "", err
	}
	if err := runGit(dir, identity, "commit", "-q", "--allow-empty", "-m", "before"); err != nil {
		return "", err
	}
	for name := range before {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return "", err
		}
	}
	if err := writeFiles(dir, after); err != nil {
		return "", err
	}
	if err := runGit(dir, nil, "add", "-A"); err != nil {
		return "", err
	}

	cmd := gitCommand(dir, env, "-c", "core.quotePath=false", "-c", "core.abbrev=10", "diff", "--cached", "-M", "-U20")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %v", strings.Join(cmd.Args, " "), err)
	}
	return string(out), nil
}

// writeFiles writes files to dir, creating parent directories as needed.
func writeFiles(dir string, files map[string]File) error {
	for name, f := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if f.Executable {
			mode = 0o755
		}
		if err := os.WriteFile(path, []byte(f.Content), mode); err != nil {
			return err
		}
	}
	return nil
}

func runGit(dir string, env []string, args ...string) error {
	cmd := gitCommand(dir, env, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %v\n%s", strings.Join(cmd.Args, " "), err, out)
	}
	return nil
}

// gitCommand returns a git command that runs in dir with the extra environment
// variables env. The command ignores the user's and the system's git
// configuration, inherited GIT_* variables and any repository above dir. It
// treats pathspecs literally.
func gitCommand(dir string, env []string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", append([]string{"--literal-pathspecs"}, args...)...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CEILING_DIRECTORIES="+filepath.Dir(dir),
	)
	cmd.Env = append(cmd.Env, env...)
	return cmd
}
