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

// Package rvecs contains functions to work with the result vectors, the internal representation
// that's used by the myers algorithm and is then translated to a user facing API. The internal
// representation is separate from the exported representation because it needs to solve a number of
// different problems.
package rvecs

import (
	"iter"

	"znkr.io/diff/internal/config"
)

// Hunk describes a sequence of consecutive edits.
type Hunk struct {
	S0, S1 int // Start and end of the hunk in x.
	T0, T1 int // Start and end of the hunk in y.
	Edits  int // Number of edits in this hunk.
}

func Hunks(rx, ry []bool, cfg config.Config) iter.Seq[Hunk] {
	return func(yield func(Hunk) bool) {
		context := cfg.Context
		s, t := 0, 0     // current index into x, y
		s0, t0 := -1, -1 // start of the current hunk
		d := 0           // number of edits in the current hunk
		run := 0         // number of consecutive matches
		n, m := len(rx)-1, len(ry)-1
		for s < n || t < m {
			if rx[s] || ry[t] {
				run = 0 // not a match, reset run counter.

				// If we're not inside a hunk, start a new hunk or, if there's an overlap due to
				// context, continue with the previous hunk.
				if s0 < 0 {
					// start of missing matches (didn't collect matches before now)
					s0, t0 = max(0, s-context), max(0, t-context)
					d = s - s0
				}

				for s < n && rx[s] {
					s++
					d++
				}
				for t < m && ry[t] {
					t++
					d++
				}
			} else {
				for s < n && t < m && !rx[s] && !ry[t] {
					s++
					t++
					run++
					d++
				}
			}
			// Active in-progress hunk and we've seen as many matches as we want in a context, finish
			// the hunk.
			if s0 >= 0 && (run > 2*context || s == n && t == m) {
				Δ := min(0, -run+context)
				if !yield(Hunk{s0, s + Δ, t0, t + Δ, d + Δ}) {
					break
				}
				s0, t0 = -1, -1
			}
		}
	}
}
