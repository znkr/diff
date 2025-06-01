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

// Package edits contains the internal edits representation that's used by the myers algorithm
// and is then translated to a user facing API.
package edits

import (
	"fmt"

	"znkr.io/diff/internal/config"
)

// Flag is a flag describing the edits for elements in both inputs.
//
// For the input slices x and y, the edit that transforms x into y is a slice edit []Flag. If
// the s-th element of x is to be deleted, edit[s]&Delete != 0 and if the t-th element of y is
// to be inserted edit[t]&Insert != 0.
type Flag uint8

const (
	None   Flag = 0
	Delete Flag = 1 << iota
	Insert
)

func (e Flag) String() string {
	switch e {
	case None:
		return "none"
	case Insert:
		return "insert"
	case Delete:
		return "delete"
	case Insert | Delete:
		return "delete|insert"
	default:
		return fmt.Sprint(uint8(e))
	}
}

// Hunk describes a sequence of consecutive edits.
type Hunk struct {
	S0, S1 int // Start and end of the hunk in x.
	T0, T1 int // Start and end of the hunk in y.
	Edits  int // Number of edits in this hunk.
}

// Hunks finds all hunks in flags and returns them.
func Hunks(flags []Flag, n, m int, cfg config.Config) (hunks []Hunk, edits int) {
	context := cfg.Context
	if n > len(flags) || m > len(flags) {
		panic("n and m must be <= len(flags)")
	}

	s, t := 0, 0     // current index into x, y
	hedits := 0      // number of edits in the current hunk
	s0, t0 := -1, -1 // start of the current hunk
	run := 0         // number of consecutive matches
	for s < n || t < m {
		del, ins := flags[s]&Delete != 0, flags[t]&Insert != 0
		if del || ins {
			run = 0 // not a match, reset run counter.

			// If we're not inside a hunk, start a new hunk or, if there's an overlap due to
			// context, continue with the previous hunk.
			if s0 < 0 {
				// start of missing matches (didn't collect matches before now)
				s0, t0 = max(0, s-context), max(0, t-context)
				hedits = s - s0

				// Check if the context windows for this new hunk and the previous hunk overlap. If
				// they do, continue filling that hunk.
				if len(hunks) > 0 && hunks[len(hunks)-1].S1 >= s0 {
					h := hunks[len(hunks)-1]
					edits -= h.Edits
					hedits = h.Edits + (s - h.S1)
					s0, t0 = h.S0, h.T0
					hunks = hunks[:len(hunks)-1]
				}
			}

			if del {
				s++
				hedits++
			}
			if ins {
				t++
				hedits++
			}
		} else {
			s++
			t++
			run++
			hedits++
		}
		// Active in-progress hunk and we've seen as many matches as we want in a context, finish
		// the hunk.
		if s0 >= 0 && (run >= context || s == n && t == m) {
			hunks = append(hunks, Hunk{s0, s, t0, t, hedits})
			s0, t0 = -1, -1
			edits += hedits
		}
	}
	return hunks, edits
}
