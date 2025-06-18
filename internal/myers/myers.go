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

// Package myers contains an implementation of Myers' algorithm.
//
// The implementation in this package uses the linear space variant described in section 4.2. In
// addition, the TOO_EXPENSIVE heuristic by Paul Eggert is used to limit the amount of time spend
// for large files with many differences.
//
// Without the heuristic, the runtime of Myers' algorithm is O(ND) where N is the sum of the length
// of both inputs and D is the number of differences. The TOO_EXPENSIVE heuristic reduces the time
// complexity to O(N^1.5 log N) but it produces suboptimal diffs.
//
// # Myers Algorithm
//
// The algorithm is a graph search on the graph modelling all possible edits that transform x to y.
// For simplicity, let's say that T is the []byte representation of string and the inputs are x =
// "ABCABBA" and y = "CBABAC". Then we can represent all possible edits from x to y with the graph:
//
//	(0,0)   A   B   C   A   B   B   A
//	    ┌───┬───┬───┬───┬───┬───┬───┐ 0
//	    │   │   │ ╲ │   │   │   │   │
//	 C  ├───┼───┼───┼───┼───┼───┼───┤ 1
//	    │   │ ╲ │   │   │ ╲ │ ╲ │   │
//	 B  ├───┼───┼───┼───┼───┼───┼───┤ 2
//	    │ ╲ │   │   │ ╲ │   │   │ ╲ │
//	 A  ├───┼───┼───┼───┼───┼───┼───┤ 3
//	    │   │ ╲ │   │   │ ╲ │ ╲ │   │
//	 B  ├───┼───┼───┼───┼───┼───┼───┤ 4
//	    │ ╲ │   │   │ ╲ │   │   │ ╲ │
//	 A  ├───┼───┼───┼───┼───┼───┼───┤ 5
//	    │   │   │ ╲ │   │   │   │   │
//	 C  └───┴───┴───┴───┴───┴───┴───┘
//	    0   1   2   3   4   5   6     (7,6)
//
// Every vertex (intersections in the graph above) corresponds to a state. The top left (0,0)
// corresponds to x and bottom right (7,6) to y.
//
// Every edge represents an edit. A step to the right represents a deletion of an element (e.g.
// moving from (0,0) to (0,1) deletes the first "A") and a step down represents an insertion (e.g.
// moving from (0,0) to (1,0) inserts a "C"). When both elements are identical, we also have
// diagonal edges representing a match.
//
// The idea behind Myers' algorithm is to find an optimal diff (fewest insertions and deletions) by
// finding a minimum-cost path from the top left (i.e. x) to the bottom right (i.e. y) where
// horizontal and vertical edges have a cost of 1 and diagonal edges have a cost of 0.
//
// Myers found a greedy algorithm with O((N+M)D) time complexity and O(D) working memory (N =
// len(x), M = len(y)). I am going to try to outline the relevant parts of the paper here without
// proofs, because they are important to understand the code below.
//
// First some nomenclature: We're going to use s and t for the horizontal and vertical coordinates
// and k for diagonals. The k=0 diagonal is the diagonal starting in (0, 0).
//
// Let a D-path be a path that has exactly D non-diagonal edges. A 0-path consists of only diagonal
// edges. By induction, it follows that a D-path must consists of a (D-1)-path plus a non-diagonal
// edge plus a possible empty sequence of diagonal edges (the paper calls a possible empty sequence
// of diagonal edges a snake, but I found this confusing and will not use that terminology here).
//
// Lemma 1: A D-path must end on diagonal k in {-D, -D+2, ..., D-2, D}.
//
// Corollary 1: A D-path ends on odd diagonals when D is odd and on even diagonals when D is even.
//
// A D-path is furthest reaching in diagonal k if and only if it is one of the D-paths ending on
// diagonal k whose end point has the greatest possible row (column) number of all such paths.
//
// Lemma 2: A furthest reaching 0-path ends at (s, s), where s is min(i-1 | x[i] != y[i] or i > M or
// i > N). A furthest reaching D-path on diagonal k can without loss of generality be decomposed
// into a furthest reaching (D-1)-path on diagonal k-1, followed by a horizontal edge, followed by
// the longest possible sequence of diagonal edges or it may be decomposed into a furthest reaching
// (D-1)-path on diagonal k+1, followed by a vertical edge, followed by the longest possible
// sequence of diagonal edges.
//
// The lemma provides us with a greedy algorithm to compute an optimal path. Unfortunately, a naive
// implementation of this algorithm as quadratic memory requirements. Fortunately, the algorithm can
// be refined to use linear memory requirements.
//
// Lemma 3: There is a a D-path from (0,0) to (N,M) if and only if there is a ⌈D/2⌉-path from (0,0)
// to some point (s,t) and a ⌊D/2⌋-path from some point (s',t') to (N,M) such that:
//
//   - (feasibility)  s'+t' >= ⌈D/2⌉ and s+t <= N+M-⌊D/2⌋, and
//   - (overlap)      s-t = s'-t' and x >= s
//
// Moreover, both D/2-paths are contained within D-paths from (0,0) to (N,M).
//
// ## References:
//
// Myers, E.W. An O(ND) difference algorithm and its variations. Algorithmica 1, 251-266 (1986).
// https://doi.org/10.1007/BF01840446
//
// The algorithm was independently discoverd by Ekko Ukkonen:
//
// Ukkonen, E. Algorithms for approximate string matching. Information and Control, Volume 64,
// Issues 1-3, 100-118 (1985). https://doi.org/10.1016/S0019-9958(85)80046-2
//
// # Heuristics
//
// TOO_EXPENSIVE: A heuristic by Paul Eggert that reduces the time complexity significantly for
// large files with many differences at the cost of suboptimal diffs. If the search for an optimal
// d-path exceeds a cost limit (in terms of d), the search is aborted and the furthest reaching
// d-path that optimizes x + y is used to determine a split.
package myers

import (
	"math"

	"znkr.io/diff/internal/config"
)

// minCostLimit is a lower bound for the TOO_EXPENSIVE heuristic. That is the heuristic is only
// applied when the cost exceeds this number (large files with a lot of differences).
const minCostLimit = 4096

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

	// First reduce the problem size by skipping all lines that are unique to x or y. Those are
	// always deletions or insertions respectively. This optimization dramatically reduces the
	// time it takes to compute very large diffs, because in practice those diffs will have many
	// lines unique to x or y.
	//
	// While we're at it, also assign a unique ID to every non-unique line to use for comparisons
	// during the application of Myers algorithm:
	//
	//  - scan x and assign a negative id to every unique line in x
	//  - scan y and change the sign of every line that also appears in y
	unique := make(map[T]int, smax-smin)
	for s := smin; s < smax; s++ {
		if unique[x[s]] == 0 {
			unique[x[s]] = -(len(unique) + 1)
		}
	}
	ny := 0
	for t := tmin; t < tmax; t++ {
		if id := unique[y[t]]; id < 0 {
			// not unique
			id = -id
			unique[y[t]] = id
			ny++
		} else if id > 0 {
			// not unique
			ny++
		}
	}
	nx := 0
	for s := smin; s < smax; s++ {
		if unique[x[s]] > 0 {
			nx++
		}
	}
	// Use the information about the unique lines to generate a subset of non-unique lines to apply
	// Myers algorithm on. If an id is > 0, the line appears in both x and y if it is <= 0 it only
	// appears in either x or y.
	buf := make([]int, 2*(nx+ny))
	var x0, y0, xidx, yidx []int
	x0, buf = buf[:0:nx], buf[nx:]
	y0, buf = buf[:0:ny], buf[ny:]
	xidx, buf = buf[:0:nx], buf[nx:]
	yidx, buf = buf[:0:ny], buf[ny:]
	if len(buf) != 0 && cap(buf) != 0 {
		panic("something went wrong during buffer assignments")
	}
	for s := smin; s < smax; s++ {
		if id := unique[x[s]]; id > 0 {
			xidx = append(xidx, s)
			x0 = append(x0, id)
		} else {
			// Unique to x, always a deletion.
			rx[s] = true
		}
	}
	for t := tmin; t < tmax; t++ {
		if id := unique[y[t]]; id > 0 {
			yidx = append(yidx, t)
			y0 = append(y0, id)
		} else {
			// Unique to y, always an insertion.
			ry[t] = true
		}
	}

	// Perform Myers algorithm on the unique IDs.
	eq := func(a, b int) bool { return a == b }
	var m myers[int]
	m.xidx, m.yidx = xidx, yidx
	m.rx, m.ry = rx, ry
	smin0, smax0, tmin0, tmax0 := m.init(x0, y0, eq)
	m.compare(smin0, smax0, tmin0, tmax0, cfg.Optimal, eq)

	return rx, ry
}

// DiffFunc compares the contents of x and y and returns the changes necessary to convert from one to
// the other.
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

type myers[T any] struct {
	// Inputs to compare.
	x, y []T

	// v-arrays for forwards and backwards iteration respectively. A v-array stores the furthest
	// reaching endpoint of a d-path in diagonal k in v[v0+k] where v0 is the offset that
	// translates k in [-d, d] to k0 = v0+k in [0, 2*d]. The endpoints only store the s-coordinate
	// since t = s - k.
	vf, vb []int
	v0     int

	// The costLimit parameter controls the TOO_EXPENSIVE heuristic that limit the runtime of
	// the algorithm for large inputs.
	costLimit int

	// Mapping of s, t indices the location in the result vectors.
	xidx, yidx []int

	// Result vectors.
	rx, ry []bool
}

func (m *myers[T]) init(x, y []T, eq func(a, b T) bool) (smin, smax, tmin, tmax int) {
	smin, tmin = 0, 0
	smax, tmax = len(x), len(y)

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

	N, M := smax-smin, tmax-tmin
	diagonals := N + M
	vlen := 2*diagonals + 3    // +1 for the middle point and +2 for the borders
	buf := make([]int, 2*vlen) // allocate space for vf and vb with a single allocation

	m.x = x
	m.y = y
	m.vf = buf[:vlen]
	m.vb = buf[vlen:]
	m.v0 = diagonals + 1 // +1 for the middle point

	// Set the costLimit to the approximate square root of the number of diagonals bounded by
	// minCostLimit.
	costLimit := 1
	for i := diagonals; i != 0; i >>= 2 {
		costLimit <<= 1
	}
	m.costLimit = max(minCostLimit, costLimit)

	if m.xidx == nil || m.yidx == nil {
		idx := make([]int, max(len(x), len(y)))
		for i := range idx {
			idx[i] = i
		}
		m.xidx = idx[:len(x)]
		m.yidx = idx[:len(y)]
	}

	if m.rx == nil || m.ry == nil {
		// For the result we add a simple border of one element that makes it easier to iterate over
		// the results.
		r := make([]bool, (len(x) + len(y) + 2))
		m.rx = r[: len(x)+1 : len(x)+1]
		m.ry = r[len(x)+1:]
	}
	return
}

// compare finds an optimal d-path from (smin, tmin) to (smax, tmax).
//
// Important: x[smin:smax] and y[tmin:tmax] must not have a common prefix or a common suffix.
func (m *myers[T]) compare(smin, smax, tmin, tmax int, optimal bool, eq func(x, y T) bool) {
	if smin == smax {
		// s is empty, therefore everything in tmin to tmax is an insertion.
		for t := tmin; t < tmax; t++ {
			m.ry[m.yidx[t]] = true
		}
	} else if tmin == tmax {
		// t is empty, therefore everything in smin to smax is a deletion.
		for s := smin; s < smax; s++ {
			m.rx[m.xidx[s]] = true
		}
	} else {
		// Use split to divide the input into three pieces:
		//
		//   (1) A, possibly empty, rect (smin, tmin) to (s0, s1)
		//   (2) A, possibly sequence of diagonals (matches) (s0, t0) to (s1, t1)
		//   (3) A, possibly empty, rect (s1, t1) to (smax, tmax)
		//
		// (1) and (3) will not have a common suffix or a common prefix, so we can use them directly
		// as inputs to compare.
		s0, s1, t0, t1, opt0, opt1 := m.split(smin, smax, tmin, tmax, optimal, eq)

		// Recurse into (1) and (3).
		m.compare(smin, s0, tmin, t0, opt0, eq)
		m.compare(s1, smax, t1, tmax, opt1, eq)
	}
}

// split finds the endpoints of a, potentially empty, sequence of diagonals in the middle of an
// optimal path from (smin, tmin) to (smax, tmax).
//
// Important: x[smin:smax] and y[tmin:tmax] must not have a common prefix or a common suffix and
// they may not both be empty.
func (m *myers[T]) split(smin, smax, tmin, tmax int, optimal bool, eq func(x, y T) bool) (s0, s1, t0, t1 int, opt0, opt1 bool) {
	N, M := smax-smin, tmax-tmin
	x, y := m.x, m.y
	vf, vb := m.vf, m.vb
	v0 := m.v0

	// Bounds for k. Since t = s - k, we an determine the min and max for k using: k = s - t.
	kmin, kmax := smin-tmax, smax-tmin

	// In contrast to the paper, we're going to number all diagonals with consistent k's by
	// centering the forwards and backwards searches around different midpoints. This way, we don't
	// need to convert k's when checking for overlap and it improves readability.
	fmid, bmid := smin-tmin, smax-tmax
	fmin, fmax := fmid, fmid
	bmin, bmax := bmid, bmid

	// We know from Corollary 1 that the optimal diff length is going to be odd or even as (N-M) is
	// odd or even. We're going to use this below to decide on when to check for path overlaps.
	odd := (N-M)%2 != 0

	// Since we can assume that split is not called with a common prefix or suffix, we know that
	// x != y, therefore there is no 0-path. Furthermore,  the d=0 iteration would result in the
	// following trivial result:
	vf[v0+fmid] = smin
	vb[v0+bmid] = smax
	// Consequently, we can start at d=1 which allows us to omit special handling of d==0 in the hot
	// k-loops below.
	//
	// We know from Lemma 3 that there's a d-path with d = ⌈N + M⌉/2. Therefore, we can omit the
	// loop condition and instead blindly increment d.
	for d := 1; ; d++ {
		// Each loop iteration, we're trying to find a d-path by first searching forwards and then
		// searching backwards for a d-path. If two paths overlap, we have found a d-path, if not
		// we're going to continue searching.

		// Forwards iteration.
		//
		// First determine which diagonals k to search. Originally, we would search k = [fmid-d,
		// fmid+d] in steps of 2, but that would lead us to move outside the edit grid and would
		// require more memory, more work, and special handling for s and t coordinates outside x
		// and y.
		//
		// Instead we put a few tighter bounds on k. We need to make sure to pick a start and end
		// point in the original search space. Since we're searching in steps of 2, this requires
		// changing the min and max for k when outside the boundary.
		//
		// Additionally, we're also initializing the v-array such that we can avoid a special case
		// in the k-loop below (for that we allocated an extra two elements up front): It let's us
		// handle the top and left hand border with the same logic as any other value.
		if fmin > kmin {
			fmin--
			vf[v0+fmin-1] = math.MinInt
		} else {
			fmin++
		}
		if fmax < kmax {
			fmax++
			vf[v0+fmax+1] = math.MinInt
		} else {
			fmax--
		}
		// The k-loop searches for the furthest reaching d-path from (0,0) to (N,M) in diagonal k.
		//
		// The v-array, v[i] = vf[v0+fmid+i] (modulo bounds on k), contains the endpoints for the
		// furthest reaching (d-1)-path in elements v[-d-1], v[-d+1], ..., v[d-1], v[d+1]. We know
		// from Lemma 1 that these elements will be disjoined from where we're going to store the
		// endpoint for the furthest reaching d-path that we're computing here.
		for k := fmin; k <= fmax; k += 2 {
			k0 := k + v0 // k as an index into vf

			// According to Lemma 2 there are two possible furthest reaching d-paths:
			//
			//   1) A furthest reaching d-path on diagonal k-1, followed by a horizontal edge,
			//      followed by the longest possible sequence of diagonals.
			//   2) A furthest reaching d-path on diagonal k+1, followed by a vertical edge,
			//      followed by the longest possible sequence of diagonals
			//
			// First find the endpoint of the furthest reaching d-path followed by a horizontal or
			// vertical edge.
			var s int
			if vf[k0-1] < vf[k0+1] {
				// Case 2. The vertical edge is implied by t = s - k.
				s = vf[k0+1]
			} else {
				// Case 1 or case 2 when v[k-1] == v[k+1]. Handling the v[k-1] == v[k+1] case
				// here prioritizes deletions over insertions.
				s = vf[k0-1] + 1
			}
			t := s - k

			// Then follow the diagonals as long as possible.
			s0, t0 := s, t
			for s < smax && t < tmax && eq(x[s], y[t]) {
				s++
				t++
			}

			// Then store the endpoint of the furthest reaching d-path.
			vf[k0] = s

			// Potentially, check for an overlap with a backwards d-path. We're done when we found
			// it.
			if odd && bmin <= k && k <= bmax && s >= vb[k0] {
				return s0, s, t0, t, true, true
			}
		}

		// Backwards iteration.
		//
		// This is mostly analogous to the forward iteration.
		if bmin > kmin {
			bmin--
			vb[v0+bmin-1] = math.MaxInt
		} else {
			bmin++
		}
		if bmax < kmax {
			bmax++
			vb[v0+bmax+1] = math.MaxInt
		} else {
			bmax--
		}
		for k := bmin; k <= bmax; k += 2 {
			k0 := k + v0
			var s int
			if vb[k0-1] < vb[k0+1] {
				s = vb[k0-1]
			} else {
				s = vb[k0+1] - 1
			}
			t := s - k

			s0, t0 := s, t
			for s > smin && t > tmin && eq(x[s-1], y[t-1]) {
				s--
				t--
			}

			vb[k0] = s

			if !odd && fmin <= k && k <= fmax && s <= vf[v0+k] {
				return s, s0, t, t0, true, true
			}
		}

		if optimal {
			continue
		}

		// Heuristic (TOO_EXPENSIVE): Limit the amount of work to find an optimal path by picking
		// a good-enough middle diagonal if we're over the cost limit.
		if d >= m.costLimit {
			// Find endpoint of the furthest reaching forward d-path that maximizes x+y.
			fbest, fbestk := math.MinInt, math.MinInt
			for k := fmin; k <= fmax; k += 2 {
				k0 := k + v0
				s := vf[k0]
				t := s - k
				if smin <= s && s < smax && tmin <= t && t < tmax && fbest < s+t {
					fbest = s + t
					fbestk = k
				}
			}

			// Find endpoint of the furthest reaching backward d-path that minimizes x+y.
			bbest, bbestk := math.MaxInt, math.MaxInt
			for k := bmin; k <= bmax; k += 2 {
				k0 := k + v0
				s := vb[k0]
				t := s - k
				if smin <= s && s < smax && tmin <= t && t < tmax && s+t < bbest {
					bbest = s + t
					bbestk = k
				}
			}

			// Use better of the two d-paths.
			if (smax+tmax)-bbest < fbest-(smin+tmin) {
				k := fbestk
				k0 := k + v0
				s := vf[k0]
				t := s - k

				// Find find the previous k, by doing the decision as in the forward iteration. And
				// use it to reconstruct the middle diagonal: By construction, the path from (s,t)
				// to (ps, pt) consists of horizontal or vertical step plus a possibly empty
				// sequence of diagonals.
				var pk int
				if vf[k0-1] < vf[k0+1] {
					pk = k + 1
				} else {
					pk = k - 1
				}
				ps := vf[pk+v0]
				pt := ps - pk
				diag := min(s-ps, t-pt)  // number of diagonal steps
				s0, t0 := s-diag, t-diag // start of diagonal
				return s0, s, t0, t, true, false
			} else {
				k := bbestk
				k0 := k + v0
				s := vb[k0]
				t := s - k

				// Analogous to forward case.
				var pk int
				if vb[k0-1] < vb[k0+1] {
					pk = k - 1
				} else {
					pk = k + 1
				}
				ps := vb[pk+v0]
				pt := ps - pk
				diag := min(ps-s, pt-t)  // number of diagonal steps
				s0, t0 := s+diag, t+diag // start of diagonal
				return s, s0, t, t0, false, true
			}
		}
	}
}
