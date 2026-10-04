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

package textdiff

import (
	"math/rand/v2"
	"testing"

	"znkr.io/diff"
	"znkr.io/diff/internal/benchdata"
)

// BenchmarkProf runs Unified with the indent heuristic on the inputs of package
// benchdata, in the modes that matter for each input.
func BenchmarkProf(b *testing.B) {
	for _, c := range []struct{ in, mode string }{
		{"small", "default"}, {"medium", "default"},
		{"large01", "default"}, {"large01", "minimal"}, {"large01", "fast"},
		{"large02", "default"}, {"large02", "minimal"}, {"large02", "fast"},
		{"large04", "default"}, {"large04", "minimal"},
		{"manifest", "default"}, {"few", "default"}, {"one", "default"},
	} {
		opts := []Option{IndentHeuristic()}
		switch c.mode {
		case "minimal":
			opts = append(opts, diff.Minimal())
		case "fast":
			opts = append(opts, diff.Fast())
		}
		b.Run(c.in+"/"+c.mode, func(b *testing.B) {
			x, y := benchdata.Load(b, c.in)
			b.ReportAllocs()
			for b.Loop() {
				_ = Unified(x, y, opts...)
			}
		})
	}
}

// BenchmarkMixed runs Unified with the indent heuristic on all inputs of
// package textdiff and internal/benchmarks, or only those under 64 KiB, which
// is the workload of a tool that diffs many files of varying size. The -io
// variants copy the inputs first, like reading them from files does.
func BenchmarkMixed(b *testing.B) {
	for _, c := range []struct {
		name    string
		maxSize int
		copy    bool
	}{
		{"all", 1 << 30, false},
		{"all-io", 1 << 30, true},
		{"small", 64 << 10, false},
		{"small-io", 64 << 10, true},
	} {
		b.Run(c.name, func(b *testing.B) {
			ps := benchdata.All(b, c.maxSize)
			b.ReportAllocs()
			for b.Loop() {
				for _, p := range ps {
					x, y := p.X, p.Y
					if c.copy {
						x, y = append([]byte(nil), x...), append([]byte(nil), y...)
					}
					_ = Unified(x, y, IndentHeuristic())
				}
			}
		})
	}
	b.Run("all-parallel", func(b *testing.B) {
		ps := benchdata.All(b, 1<<30)
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := rand.IntN(len(ps))
			for pb.Next() {
				p := ps[i%len(ps)]
				_ = Unified(p.X, p.Y, IndentHeuristic())
				i++
			}
		})
	})
}
