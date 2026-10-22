// Copyright 2026 Florian Zenker (flo@znkr.io)
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

//go:build go1.27

package impl

import (
	"hash/maphash"
	"testing"

	"znkr.io/diff/internal/config"
)

func TestDiffHashMatchesDiff(t *testing.T) {
	// DiffHash with maphash.ComparableHasher hashes and compares with ==, the
	// same as Diff, so both must return the same result.
	for _, in := range unusualInputs() {
		checkDiffHashMatchesDiff(t, in.fx, in.fy)
		checkDiffHashMatchesDiff(t, in.ax, in.ay)
		checkDiffHashMatchesDiff(t, in.px, in.py)
	}
}

func checkDiffHashMatchesDiff[T comparable](t *testing.T, x, y []T) {
	t.Helper()
	for _, cfg := range []config.Config{
		{Mode: config.ModeMinimal},
		{Mode: config.ModeDefault},
		{Mode: config.ModeDefault, ForceAnchoringHeuristic: true},
		{Mode: config.ModeFast},
	} {
		rx, ry := Diff(x, y, cfg)
		hx, hy := DiffHash(x, y, maphash.ComparableHasher[T]{}, cfg)
		for i := range len(x) {
			if rx.Get(i) != hx.Get(i) {
				t.Fatalf("DiffHash(%v, %v, %+v) differs from Diff at x[%d]", x, y, cfg, i)
			}
		}
		for j := range len(y) {
			if ry.Get(j) != hy.Get(j) {
				t.Fatalf("DiffHash(%v, %v, %+v) differs from Diff at y[%d]", x, y, cfg, j)
			}
		}
	}
}

func TestDiffHashMinimal(t *testing.T) {
	// In minimal mode, the number of edits is the number of elements that
	// aren't part of a longest common subsequence.
	cfg := config.Config{Mode: config.ModeMinimal}
	for _, in := range minimalInputs() {
		x, y := in[0], in[1]
		want := len(x) + len(y) - 2*lcsLen(x, y)
		rx, ry := DiffHash(x, y, maphash.ComparableHasher[int]{}, cfg)
		if got := countEdits(t, x, y, rx, ry); got != want {
			t.Fatalf("DiffHash(%v, %v) has %d edits, want %d", x, y, got, want)
		}
	}
}
