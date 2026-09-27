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

package rvecs

import (
	"znkr.io/diff/internal/config"
)

// Hunk describes a sequence of consecutive edits.
type Hunk struct {
	S0, S1 int // Start and end of the hunk in x.
	T0, T1 int // Start and end of the hunk in y.
	Edits  int // Number of edits in this hunk.
}

// AppendHunks appends the hunks of the result vectors rx and ry to dst and
// returns the result.
func AppendHunks(dst []Hunk, rx, ry Vec, cfg config.Config) []Hunk {
	context := cfg.Context
	s, t := 0, 0     // current index into x, y
	s0, t0 := -1, -1 // start of the current hunk
	d := 0           // number of edits in the current hunk
	run := 0         // number of consecutive matches
	n, m := rx.Len()-1, ry.Len()-1
	for s < n || t < m {
		if rx.Get(s) || ry.Get(t) {
			run = 0 // not a match, reset run counter.

			// If we're not inside a hunk, start a new hunk or, if there's an
			// overlap due to context, continue with the previous hunk.
			if s0 < 0 {
				// start of missing matches (didn't collect matches before now)
				s0, t0 = max(0, s-context), max(0, t-context)
				d = s - s0
			}

			// Skip the changes in x and y. The element at n (m) is unchanged.
			e, f := rx.NextClear(s), ry.NextClear(t)
			d += (e - s) + (f - t)
			s, t = e, f
		} else {
			// Skip to the next change in x or y, or to the end of either.
			k := min(min(rx.NextSet(s), n)-s, min(ry.NextSet(t), m)-t)
			s += k
			t += k
			run += k
			d += k
		}
		// Active in-progress hunk and we've seen as many matches as we want in
		// a context, finish the hunk.
		if s0 >= 0 && (run > 2*context || s == n && t == m) {
			Δ := min(0, -run+context)
			dst = append(dst, Hunk{s0, s + Δ, t0, t + Δ, d + Δ})
			s0, t0 = -1, -1
		}
	}
	return dst
}
