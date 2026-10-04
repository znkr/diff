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

package diff

import (
	"strings"
	"testing"

	"znkr.io/diff/internal/benchdata"
)

// genericInputs lists the inputs of package benchdata that BenchmarkGeneric and
// BenchmarkGenericFunc use.
var genericInputs = []string{"small", "medium", "large01", "large02", "few", "manifest"}

// BenchmarkGeneric runs Hunks on the lines of inputs of package benchdata as
// []string.
func BenchmarkGeneric(b *testing.B) {
	for _, name := range genericInputs {
		b.Run(name, func(b *testing.B) {
			xs, ys := benchLines(b, name)
			b.ReportAllocs()
			for b.Loop() {
				_ = Hunks(xs, ys)
			}
		})
	}
}

// BenchmarkGenericFunc runs HunksFunc on the lines of inputs of package
// benchdata as []string.
func BenchmarkGenericFunc(b *testing.B) {
	eq := func(a, b string) bool { return a == b }
	for _, name := range genericInputs {
		b.Run(name, func(b *testing.B) {
			xs, ys := benchLines(b, name)
			b.ReportAllocs()
			for b.Loop() {
				_ = HunksFunc(xs, ys, eq)
			}
		})
	}
}

// benchLines returns the lines of the inputs of package benchdata with the
// given name.
func benchLines(b *testing.B, name string) (xs, ys []string) {
	x, y := benchdata.Load(b, name)
	return strings.SplitAfter(string(x), "\n"), strings.SplitAfter(string(y), "\n")
}
