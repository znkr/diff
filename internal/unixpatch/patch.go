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

// Package unixpatch provides a simple wrapper around the unix patch tool.
//
// This package is only for testing.
package unixpatch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Patch(orig, diff string) (string, error) {
	// Using patch with an empty diff will not create an output file.
	if len(diff) == 0 {
		return orig, nil
	}

	dir, err := os.MkdirTemp("", "patch-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(dir)

	patchfile := filepath.Join(dir, "patch")
	origfile := filepath.Join(dir, "orig")
	outfile := filepath.Join(dir, "out")

	if err := os.WriteFile(patchfile, []byte(diff), 0o644); err != nil {
		return "", fmt.Errorf("failed to write patch file: %v", err)
	}
	if err := os.WriteFile(origfile, []byte(orig), 0o644); err != nil {
		return "", fmt.Errorf("failed to write orig file: %v", err)
	}

	cmd := exec.Command("patch", "-u", "-i", patchfile, "-o", outfile, origfile)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to run patch command: patch %s: %v\n%s", strings.Join(cmd.Args, " "), err, out)
	}

	out, err := os.ReadFile(outfile)
	if err != nil {
		return "", fmt.Errorf("failed to read outfile: %v", err)
	}

	return string(out), nil
}
