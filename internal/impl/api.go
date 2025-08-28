// Copyright 2025 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// The segments function is derived from Go's src/internal/diff/diff.go
// which has the following copyright and license:
//
// Copyright 2022 The Go Authors. All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//    * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//    * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//    * Neither the name of Google LLC nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package impl

import (
	"sort"

	"znkr.io/diff/internal/config"
)

// Diff compares the contents of x and y and returns the changes necessary to convert from one to
// the other.
func Diff[T comparable](x, y []T, cfg config.Config) (rx, ry []bool) {
	smin, tmin := 0, 0
	smax, tmax := len(x), len(y)

	// Strip common prefix.
	for smin < smax && tmin < tmax && x[smin] == y[tmin] {
		smin++
		tmin++
	}

	// Strip common suffix.
	for smax > smin && tmax > tmin && x[smax-1] == y[tmax-1] {
		smax--
		tmax--
	}

	// Allocate result vectors.
	r := make([]bool, (len(x) + len(y) + 2))
	rx = r[: len(x)+1 : len(x)+1]
	ry = r[len(x)+1:]

	// Handle trivial cases without doing anything extra.
	switch {
	case smin != smax && tmin == tmax:
		for s := smin; s < smax; s++ {
			rx[s] = true
		}
		return rx, ry
	case smin == smax && tmin != tmax:
		for t := tmin; t < tmax; t++ {
			ry[t] = true
		}
		return rx, ry
	case smin == smax && tmin == tmax:
		return rx, ry
	}

	// The code block below is an important optimization that significantly reduces the problem size
	// and time complexity.
	//
	// For performance reasons, it's doing a number of things at once. This makes it quite hard to
	// follow. To understand it, it's necessary to understand the individual tasks:
	//
	// Assign a unique ID to every unique input element in x and y that appears in both x and y.
	// This allows us to apply Myers' algorithm on integers instead of T (for faster comparison and
	// specialized implementation) and provides a dense ID space that makes it possible to use a
	// slice instead of a map to efficiently determine which elements exist in both x and y.
	//
	// Drop all elements that only appear in x or y. These are always deletions and insertions
	// respectively. This optimization dramatically reduces the time it takes to compute very large
	// diffs, because in practice those diffs will have many lines unique to x or y.
	//
	// Find all anchors, that is all elements that appear exactly once in interesting part of x and
	// y (x[smin:smax], y[tmin:tmax]). We do that by counting the number of occurrences as 0, 1,
	// many for both x and y. Using 0, 1, 2 for counts of elements in x and 0, 4, 8 for counts of
	// elements in y. For elements in y, we only count elements that were already found in x. With
	// that, a count > 4 means the element appears in both x and y and a count = 1+4 means the
	// element is an anchor.
	//
	// The results are the following slices:
	//  * x0:     x[smin:smax] in as IDs except for elements that appear only in x
	//  * y0:     y[tmin:tmax] in as IDs except for elements that appear only in y
	//  * xidx:   A mapping from x0 to x: x0[s] corresponds to x[xidx[s]]
	//  * yidx:   A mapping from y0 to y: y0[t] corresponds to y[yidx[t]]
	//  * counts: The number of times a ID appears in x and y.
	//
	// Note: The code below is trading some density of the ID space (and with that memory) for
	// improved runtime. The bottleneck here are map lookups, the code below is structured so that
	// the number of map lookups is minimal.
	idx := make(map[T]int, smax-smin) // temporary map from element to ID
	var x0, y0, xidx, yidx []int
	buf := make([]int, 2*(smax-smin)+2*(tmax-tmin))
	x0, buf = buf[:0:smax-smin], buf[smax-smin:]
	xidx, buf = buf[:0:smax-smin], buf[smax-smin:]
	y0, buf = buf[:0:tmax-tmin], buf[tmax-tmin:]
	yidx, buf = buf[:0:tmax-tmin], buf[tmax-tmin:]
	if len(buf) != 0 && cap(buf) != 0 {
		panic("something went wrong during buffer assignments")
	}
	counts := make([]int, smax-smin)
	// Step 1: Create an ID for every element in x[smin:smax] and count the number of occurrences.
	for _, e := range x[smin:smax] {
		id, ok := idx[e]
		if !ok {
			id = len(idx)
			idx[e] = id
		}
		if c := counts[id]; c < 2 {
			counts[id] = c + 1
		}
		x0 = append(x0, id)
	}
	// Step 2: Do the same for y, but already ignore everything that's not in x, except for marking
	// these elements as insertions.
	for i, e := range y[tmin:tmax] {
		id, ok := idx[e]
		if !ok {
			// Not in x, this is always an insertion.
			ry[i+tmin] = true
			continue
		}
		if c := counts[id]; c < 8 {
			counts[id] = c + 4
		}
		yidx = append(yidx, i+tmin)
		y0 = append(y0, id)
	}
	// Step 3: Filter out elements from x0 that are not in y.
	nanchors := 0
	i := 0
	for j, e := range x0 {
		if c := counts[e]; c > 4 {
			xidx = append(xidx, j+smin)
			x0[i] = e
			if c == 1+4 {
				// Element appears exactly once in x (1) and y (4).
				nanchors++
			}
			i++
		} else {
			rx[j+smin] = true // always an deletion
		}
	}
	x0 = x0[:i]

	// Perform Myers algorithm on the unique IDs.
	var m myersInt
	m.xidx, m.yidx = xidx, yidx
	m.rx, m.ry = rx, ry
	smin0, smax0, tmin0, tmax0 := m.init(x0, y0)

	switch {
	case cfg.Optimal:
		fallthrough
	default:
		m.compare(smin0, smax0, tmin0, tmax0, cfg.Optimal)

	// Heuristic (ANCHORING): If the input is too large and we have found anchors, use the anchoring
	// heuristic. This provides a significant performance boost and provides more optimal results
	// than the other heuristics.
	case nanchors > 0 && (smax0-smin0)+(tmax0-tmin0) > anchoringHeuristicMinInputLen:
		fallthrough
	case cfg.ForceAnchoringHeuristic:
		segments := segments(smin0, smax0, tmin0, tmax0, nanchors, counts, x0, y0)
		done := segments[0]
		for _, anchor := range segments[1:] {
			if anchor.s < done.s {
				// Already handled scanning forward from earlier match.
				continue
			}

			start := anchor
			for start.s > done.s && start.t > done.t && x0[start.s-1] == y0[start.t-1] {
				start.s--
				start.t--
			}
			end := anchor
			for end.s < smax0 && end.t < tmax0 && x0[end.s] == y0[end.t] {
				end.s++
				end.t++
			}

			m.compare(done.s, start.s, done.t, start.t, cfg.Optimal)

			if end.s >= smax0 && end.t >= tmax0 {
				break
			}

			done = end
		}
	}

	return rx, ry
}

type pair struct{ s, t int }

// segments returns the pairs of indexes of the longest common subsequence of anchors in x and y.
//
// The longest common subsequence algorithm is as described in Thomas G. Szymanski, “A Special Case
// of the Maximal Common Subsequence Problem,” Princeton TR #170 (January 1975), available at
// https://research.swtch.com/tgs170.pdf.
func segments(smin, smax, tmin, tmax int, nanchors int, counts []int, x, y []int) []pair {
	idx := make(map[int]int, nanchors)
	buf := make([]int, 3*nanchors)
	var xi, yi, inv []int
	xi, buf = buf[:0:nanchors], buf[nanchors:]
	yi, buf = buf[:0:nanchors], buf[nanchors:]
	inv, buf = buf[:0:nanchors], buf[nanchors:]
	if len(buf) != 0 && cap(buf) != 0 {
		panic("something went wrong during buffer assignments")
	}

	// Gather the indices of anchors in x and y:
	//	xi[i] = increasing indexes of unique strings in x.
	//	yi[i] = increasing indexes of unique strings in y.
	//	inv[i] = index j such that x[xi[i]] = y[yi[j]].
	for i, e := range y[tmin:tmax] {
		t := tmin + i
		if counts[e] == 1+4 {
			idx[e] = len(yi)
			yi = append(yi, t)
		}
	}
	for i, e := range x[smin:smax] {
		s := smin + i
		if counts[e] == 1+4 {
			xi = append(xi, s)
			inv = append(inv, idx[e])
		}
	}

	// Apply Algorithm A from Szymanski's paper.
	// In those terms, A = J = inv and B = [0, n).
	// We add sentinel pairs {0,0}, and {len(x),len(y)}
	// to the returned sequence, to help the processing loop.
	J := inv
	n := len(xi)
	T := make([]int, n)
	L := make([]int, n)
	for i := range T {
		T[i] = n + 1
	}
	for i := range n {
		k := sort.Search(n, func(k int) bool {
			return T[k] >= J[i]
		})
		T[k] = J[i]
		L[i] = k + 1
	}
	k := 0
	for _, v := range L {
		if k < v {
			k = v
		}
	}
	anchors := make([]pair, 2+k)
	anchors[1+k] = pair{smax, tmax} // sentinel at end
	lastj := n
	for i := n - 1; i >= 0; i-- {
		if L[i] == k && J[i] < lastj {
			anchors[k] = pair{xi[i], yi[J[i]]}
			k--
		}
	}
	anchors[0] = pair{smin, tmin} // sentinel at start
	return anchors
}

// DiffFunc compares the contents of x and y and returns the changes necessary to convert from one
// to the other.
//
// Note that this function has generally worse performance than [Diff] for diffs with many changes.
func DiffFunc[T any](x, y []T, eq func(a, b T) bool, cfg config.Config) (rx, ry []bool) {
	smin, tmin := 0, 0
	smax, tmax := len(x), len(y)

	// Strip common prefix.
	for smin < smax && tmin < tmax && eq(x[smin], y[tmin]) {
		smin++
		tmin++
	}

	// Strip common suffix.
	for smax > smin && tmax > tmin && eq(x[smax-1], y[tmax-1]) {
		smax--
		tmax--
	}

	// Allocate result vectors.
	r := make([]bool, (len(x) + len(y) + 2))
	rx = r[: len(x)+1 : len(x)+1]
	ry = r[len(x)+1:]

	// Handle trivial cases without doing anything extra.
	switch {
	case smin != smax && tmin == tmax:
		for s := smin; s < smax; s++ {
			rx[s] = true
		}
		return rx, ry
	case smin == smax && tmin != tmax:
		for t := tmin; t < tmax; t++ {
			ry[t] = true
		}
		return rx, ry
	case smin == smax && tmin == tmax:
		return rx, ry
	}

	var m myers[T]
	m.rx, m.ry = rx, ry
	smin, smax, tmin, tmax = m.init(x, y, eq)
	m.compare(smin, smax, tmin, tmax, cfg.Optimal, eq)
	return m.rx, m.ry
}
