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

// Package benchdata provides the inputs of the benchmarks in packages diff and
// textdiff.
package benchdata

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"
)

// Names lists the inputs that [Load] provides.
var Names = []string{"small", "medium", "large01", "large02", "large04", "manifest", "few", "one"}

var files = map[string]string{
	"small":    "internal/benchmarks/testdata/small.test",
	"medium":   "internal/benchmarks/testdata/medium.test",
	"large01":  "internal/benchmarks/testdata/large_01.test",
	"large02":  "internal/benchmarks/testdata/large_02.test",
	"large04":  "internal/benchmarks/testdata/large_04.test",
	"manifest": "textdiff/testdata/go_78eadf5b3de568297456fe137b65ff16e8cc8bb6_src_cmd_vendor_golang.org_x_tools_internal_stdlib_manifest.go.test",
}

// Load returns the inputs x and y with the given name, one of [Names]. "few" is
// x of large04 with 20 lines edited, and "one" is x of large04 with 1 line
// edited.
func Load(tb testing.TB, name string) (x, y []byte) {
	tb.Helper()
	switch name {
	case "few":
		return edit(tb, 20)
	case "one":
		return edit(tb, 1)
	}
	file, ok := files[name]
	if !ok {
		tb.Fatalf("unknown input %q", name)
	}
	return parse(tb, filepath.Join(root(tb), file))
}

// edit returns x of large04 and a copy of it with n lines edited.
func edit(tb testing.TB, n int) (x, y []byte) {
	x, _ = Load(tb, "large04")
	lines := strings.SplitAfter(string(x), "\n")
	if n == 1 {
		lines[len(lines)/2] = "// edited " + lines[len(lines)/2]
	} else {
		for i := 1; i <= n; i++ {
			j := i * len(lines) / (n + 1)
			lines[j] = "// edited " + lines[j]
		}
	}
	return x, []byte(strings.Join(lines, ""))
}

// Pair holds the inputs of a diff.
type Pair struct{ X, Y []byte }

// All returns the inputs of the test cases of package textdiff and of
// internal/benchmarks with at most maxSize bytes, in a fixed pseudorandom
// order.
func All(tb testing.TB, maxSize int) []Pair {
	tb.Helper()
	dir := root(tb)
	files, _ := filepath.Glob(filepath.Join(dir, "textdiff/testdata/*.test"))
	more, _ := filepath.Glob(filepath.Join(dir, "internal/benchmarks/testdata/*.test"))
	var ps []Pair
	for _, f := range append(files, more...) {
		x, y := parse(tb, f)
		if len(x)+len(y) <= maxSize {
			ps = append(ps, Pair{x, y})
		}
	}
	rand.New(rand.NewPCG(1, 2)).Shuffle(len(ps), func(i, j int) { ps[i], ps[j] = ps[j], ps[i] })
	return ps
}

// parse returns the files x and y of the txtar archive at path.
func parse(tb testing.TB, path string) (x, y []byte) {
	tb.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	for _, f := range txtar.Parse(data).Files {
		switch f.Name {
		case "x":
			x = f.Data
		case "y":
			y = f.Data
		}
	}
	return x, y
}

// root returns the root directory of the repository: the closest directory at
// or above the working directory with the go.mod file of module znkr.io/diff.
func root(tb testing.TB) string {
	tb.Helper()
	// The inputs are in separate modules, so that the module znkr.io/diff
	// doesn't include them. Tests run in the directory of their package, which
	// works with -trimpath, unlike the path of this source file.
	dir, err := os.Getwd()
	if err != nil {
		tb.Fatal(err)
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && moduleLine.Match(data) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			tb.Fatal("benchdata: can't find the repository of module znkr.io/diff")
		}
		dir = parent
	}
}

var moduleLine = regexp.MustCompile(`(?m)^module znkr\.io/diff\s*$`)
