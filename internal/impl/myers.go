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

package impl

import (
	"math"

	"znkr.io/diff/internal/pool"
	"znkr.io/diff/internal/rvecs"
)

type myers[T any] struct {
	// Inputs to compare.
	x, y []T

	// v-arrays for forwards and backwards iteration respectively. A v-array
	// stores the furthest reaching endpoint of a d-path in diagonal k in
	// v[v0+k] where v0 is the offset that translates k in [-M-1, N+1] to k0 =
	// v0+k in [0, N+M+2]. The endpoints only store the s-coordinate since t = s
	// - k.
	vf, vb []int
	v0     int

	// The costLimit parameter controls the TOO_EXPENSIVE heuristic that limit
	// the runtime of the algorithm for large inputs.
	costLimit int

	// Mapping of s, t indices the location in the result vectors. If they
	// are nil, s and t are the locations.
	xidx, yidx []int

	// Result vectors.
	rx, ry rvecs.Vec
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
	// init strips the same number of elements from the start of x and y, so
	// every split has k = s - t in [-M, N]. The two extra elements hold the
	// borders at k = -M-1 and k = N+1.
	vlen := diagonals + 3
	buf := pool.Ints.Get(2 * vlen) // space for vf and vb in a single slice

	m.x = x
	m.y = y
	m.vf = buf[:vlen]
	m.vb = buf[vlen:]
	m.v0 = M + 1

	// Set the costLimit to the approximate square root of the number of
	// diagonals bounded by minCostLimit.
	costLimit := 1
	for i := diagonals; i != 0; i >>= 2 {
		costLimit <<= 1
	}
	m.costLimit = max(minCostLimit, costLimit)

	if m.rx.Len() == 0 || m.ry.Len() == 0 {
		m.rx, m.ry = rvecs.Make(len(x), len(y))
	}
	return
}

// release returns the memory of the v-arrays to [pool.Ints]. m must not be used
// after release.
func (m *myers[T]) release() {
	// init allocates vf and vb as one slice with vb directly after vf.
	pool.Ints.Put(m.vf[:cap(m.vf)])
}

// compare finds an optimal d-path from (smin, tmin) to (smax, tmax).
func (m *myers[T]) compare(smin, smax, tmin, tmax int, optimal bool, eq func(x, y T) bool) {
	// split requires inputs without a common prefix or suffix. The pieces
	// before and after the diagonal that split returns can have one, because a
	// match can precede or follow the diagonal without being part of it.
	for smin < smax && tmin < tmax && eq(m.x[smin], m.y[tmin]) {
		smin++
		tmin++
	}
	for smax > smin && tmax > tmin && eq(m.x[smax-1], m.y[tmax-1]) {
		smax--
		tmax--
	}

	if smin == smax {
		// s is empty, therefore everything in tmin to tmax is an insertion.
		if m.yidx == nil {
			m.ry.SetRange(tmin, tmax)
		} else {
			m.ry.SetSorted(m.yidx[tmin:tmax])
		}
	} else if tmin == tmax {
		// t is empty, therefore everything in smin to smax is a deletion.
		if m.xidx == nil {
			m.rx.SetRange(smin, smax)
		} else {
			m.rx.SetSorted(m.xidx[smin:smax])
		}
	} else {
		// Use split to divide the input into three pieces:
		//
		//   (1) A, possibly empty, rect (smin, tmin) to (s0, t0)
		//   (2) A, possibly empty, sequence of diagonals (matches) (s0, t0) to
		//       (s1, t1)
		//   (3) A, possibly empty, rect (s1, t1) to (smax, tmax)
		s0, s1, t0, t1, opt0, opt1 := m.split(smin, smax, tmin, tmax, optimal, eq)

		// Recurse into (1) and (3).
		m.compare(smin, s0, tmin, t0, opt0, eq)
		m.compare(s1, smax, t1, tmax, opt1, eq)
	}
}

// split finds the endpoints of a, potentially empty, sequence of diagonals in
// the middle of an optimal path from (smin, tmin) to (smax, tmax).
//
// Important: x[smin:smax] and y[tmin:tmax] must not have a common prefix or a
// common suffix and they may not both be empty.
func (m *myers[T]) split(smin, smax, tmin, tmax int, optimal bool, eq func(x, y T) bool) (s0, s1, t0, t1 int, opt0, opt1 bool) {
	N, M := smax-smin, tmax-tmin
	vf, vb := m.vf, m.vb
	v0 := m.v0

	// Bounds for k. Since t = s - k, we an determine the min and max for k
	// using: k = s - t.
	kmin, kmax := smin-tmax, smax-tmin

	// In contrast to the paper, we're going to number all diagonals with
	// consistent k's by centering the forwards and backwards searches around
	// different midpoints. This way, we don't need to convert k's when checking
	// for overlap and it improves readability.
	fmid, bmid := smin-tmin, smax-tmax
	fmin, fmax := fmid, fmid
	bmin, bmax := bmid, bmid

	// We know from Corollary 1 that the optimal diff length is going to be odd
	// or even as (N-M) is odd or even. We're going to use this below to decide
	// on when to check for path overlaps.
	odd := (N-M)%2 != 0

	// Since we can assume that split is not called with a common prefix or
	// suffix, we know that x != y, therefore there is no 0-path. Furthermore,
	// the d=0 iteration would result in the following trivial result:
	vf[v0+fmid] = smin
	vb[v0+bmid] = smax
	// Consequently, we can start at d=1 which allows us to omit special
	// handling of d==0 in the hot k-loops below.
	//
	// We know from Lemma 3 that there's a d-path with d = ⌈N + M⌉/2. Therefore,
	// we can omit the loop condition and instead blindly increment d.
	for d := 1; ; d++ {
		// Each loop iteration, we're trying to find a d-path by first searching
		// forwards and then searching backwards for a d-path. If two paths
		// overlap, we have found a d-path, if not we're going to continue
		// searching.

		// Forwards iteration.
		//
		// First determine which diagonals k to search. Originally, we would
		// search k = [fmid-d, fmid+d] in steps of 2, but that would lead us to
		// move outside the edit grid and would require more memory, more work,
		// and special handling for s and t coordinates outside x and y.
		//
		// Instead we put a few tighter bounds on k. We need to make sure to
		// pick a start and end point in the original search space. Since we're
		// searching in steps of 2, this requires changing the min and max for k
		// when outside the boundary.
		//
		// Additionally, we're also initializing the v-array such that we can
		// avoid a special case in the k-loop below (for that we allocated an
		// extra two elements up front): It let's us handle the top and left
		// hand border with the same logic as any other value.
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
		fs0, fs1, ft0, ft1, flongest, found := m.forward(fmin, fmax, bmin, bmax, smax, tmax, odd, eq)
		if found {
			return fs0, fs1, ft0, ft1, true, true
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
		bs0, bs1, bt0, bt1, blongest, found := m.backward(bmin, bmax, fmin, fmax, smin, tmin, !odd, eq)
		if found {
			return bs0, bs1, bt0, bt1, true, true
		}
		longestDiag := max(flongest, blongest) // Longest diagonal we found

		if optimal {
			continue
		}

		// Heuristic (GOOD_DIAGONAL): If we're over the cost limit for this
		// heuristic, we accept a good diagonal to split the search space
		// instead of searching for the optimal split point.
		//
		// A good diagonal is one that's longer than goodDiagMinLen, not too far
		// from a corner and not too far from the middle diagonal.
		if longestDiag >= goodDiagMinLen && d >= goodDiagCostLimit {
			best := struct {
				v              int
				s0, s1, t0, t1 int
				opt0, opt1     bool
			}{}
			// Check forward paths.
			for k := fmin; k <= fmax; k += 2 {
				k0 := k + v0
				s := vf[k0]
				t := s - k
				// v is the progress of the path minus the distance of its
				// diagonal from the middle diagonal.
				v := (s - smin) + (t - tmin) - max(fmid-k, k-fmid)
				if s < smin || smax <= s || t < tmin || tmax <= t {
					continue
				}
				if v <= goodDiagMagic*d || v < best.v {
					continue // not good enough, check next diagonal
				}

				// Find find the previous k, by doing the decision as in the
				// forward iteration. And use it to reconstruct the middle
				// diagonal: By construction, the path from (s,t) to (ps, pt)
				// consists of horizontal or vertical step plus a possibly empty
				// sequence of diagonals.
				var pk int
				if vf[k0-1] < vf[k0+1] {
					pk = k + 1
				} else {
					pk = k - 1
				}
				ps := vf[pk+v0]
				pt := ps - pk
				diag := min(s-ps, t-pt) // number of diagonal steps
				if diag >= goodDiagMinLen {
					best.v = v
					best.s0 = s - diag
					best.s1 = s
					best.t0 = t - diag
					best.t1 = t
					best.opt0 = true
					best.opt1 = false
				}
			}
			// Check backward paths.
			for k := bmin; k <= bmax; k += 2 {
				k0 := k + v0
				s := vb[k0]
				t := s - k
				if s < smin || smax <= s || t < tmin || tmax <= t {
					continue
				}
				v := (smax - s) + (tmax - t) - max(bmid-k, k-bmid)
				if v <= goodDiagMagic*d || v < best.v {
					continue
				}

				var pk int
				if vb[k0-1] < vb[k0+1] {
					pk = k - 1
				} else {
					pk = k + 1
				}
				ps := vb[pk+v0]
				pt := ps - pk
				diag := min(ps-s, pt-t) // number of diagonal steps
				if diag >= goodDiagMinLen {
					best.v = v
					best.s0 = s
					best.s1 = s + diag
					best.t0 = t
					best.t1 = t + diag
					best.opt0 = false
					best.opt1 = true
				}
			}
			if best.v > 0 {
				return best.s0, best.s1, best.t0, best.t1, best.opt0, best.opt1
			}
		}

		// Heuristic (TOO_EXPENSIVE): Limit the amount of work to find an
		// optimal path by picking a good-enough middle diagonal if we're over
		// the cost limit.
		if d >= m.costLimit {
			// Find endpoint of the furthest reaching forward d-path that
			// maximizes x+y.
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

			// Find endpoint of the furthest reaching backward d-path that
			// minimizes x+y.
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
			if fbest != math.MinInt && (smax+tmax)-bbest < fbest-(smin+tmin) {
				k := fbestk
				k0 := k + v0
				s := vf[k0]
				t := s - k

				// Same as in GOOD_DIAGONAL heuristic.
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
			} else if bbest != math.MaxInt {
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
			} else {
				panic("no best path found")
			}
		}
	}
}

// forward extends the furthest reaching forward paths on diagonals fmin,
// fmin+2, ..., fmax by one edit and the longest possible sequence of diagonals.
// It returns the start and end of the diagonals (s0, s1, t0, t1) of the first
// path that overlaps a backward path on a diagonal in [bmin, bmax] and found =
// true, if check is set and there is one. longest is the length of the longest
// sequence of diagonals.
func (m *myers[T]) forward(fmin, fmax, bmin, bmax, smax, tmax int, check bool, eq func(x, y T) bool) (s0, s1, t0, t1, longest int, found bool) {
	x, y := m.x[:smax], m.y[:tmax]
	// wf and wb hold the entries of vf and vb for the diagonals fmin-1 to
	// fmax+1; the entry for diagonal k is at w = k-fmin+1. Looping over w lets
	// the compiler drop the bounds checks.
	lo, hi := m.v0+fmin-1, m.v0+fmax+2
	wf := m.vf[lo:hi]
	wb := m.vb[lo:hi][:len(wf)]
	// The k-loop searches for the furthest reaching d-path from (0,0) to (N,M)
	// in diagonal k.
	//
	// The v-array, v[i] = vf[v0+fmid+i] (modulo bounds on k), contains the
	// endpoints for the furthest reaching (d-1)-path in elements v[-d-1],
	// v[-d+1], ..., v[d-1], v[d+1]. We know from Lemma 1 that these elements
	// will be disjoined from where we're going to store the endpoint for the
	// furthest reaching d-path that we're computing here.
	for w := 1; w < len(wf)-1; w += 2 {
		k := fmin - 1 + w

		// According to Lemma 2 there are two possible furthest reaching
		// d-paths:
		//
		//   1) A furthest reaching d-path on diagonal k-1, followed by a
		//      horizontal edge, followed by the longest possible sequence of
		//      diagonals.
		//   2) A furthest reaching d-path on diagonal k+1, followed by a
		//      vertical edge, followed by the longest possible sequence of
		//      diagonals
		//
		// First find the endpoint of the furthest reaching d-path followed by a
		// horizontal or vertical edge.
		//
		// Case 1 ends at wf[w-1]+1 and case 2 at wf[w+1]. If wf[w-1] ==
		// wf[w+1], case 1 wins, which prioritizes deletions over insertions.
		s := max(wf[w-1]+1, wf[w+1])
		t := s - k

		// Then follow the diagonals as long as possible.
		//
		// s and t are never negative. Comparing them as uint lets the compiler
		// drop the bounds checks for x[s] and y[t].
		s0, t0 := s, t
		for uint(s) < uint(len(x)) && uint(t) < uint(len(y)) && eq(x[s], y[t]) {
			s++
			t++
		}

		// If we have found a long diagonal, we may be able to apply the
		// GOOD_DIAGONAL heuristic (see below).
		longest = max(longest, s-s0)

		// Then store the endpoint of the furthest reaching d-path.
		wf[w] = s

		// Potentially, check for an overlap with a backwards d-path. We're done
		// when we found it.
		if check && bmin <= k && k <= bmax && s >= wb[w] {
			return s0, s, t0, t, longest, true
		}
	}
	return 0, 0, 0, 0, longest, false
}

// backward is the analog of [myers.forward] for the backward paths on diagonals
// bmin, bmin+2, ..., bmax. It checks for an overlap with a forward path on a
// diagonal in [fmin, fmax].
func (m *myers[T]) backward(bmin, bmax, fmin, fmax, smin, tmin int, check bool, eq func(x, y T) bool) (s0, s1, t0, t1, longest int, found bool) {
	xb, yb := m.x[smin:], m.y[tmin:]
	// wb and wf hold the entries of vb and vf for the diagonals bmin-1 to
	// bmax+1; the entry for diagonal k is at w = k-bmin+1. Looping over w lets
	// the compiler drop the bounds checks.
	lo, hi := m.v0+bmin-1, m.v0+bmax+2
	wb := m.vb[lo:hi]
	wf := m.vf[lo:hi][:len(wb)]
	for w := 1; w < len(wb)-1; w += 2 {
		k := bmin - 1 + w
		s := min(wb[w-1], wb[w+1]-1)
		t := s - k

		// The snake compares xb[i-1] and yb[j-1] with i = s-smin and j =
		// t-tmin. s <= len(m.x) and t <= len(m.y), so comparing i-1 and j-1 as
		// uint checks s > smin and t > tmin and lets the compiler drop the
		// bounds checks.
		s0, t0 := s, t
		i, j := s-smin, t-tmin
		for uint(i-1) < uint(len(xb)) && uint(j-1) < uint(len(yb)) && eq(xb[i-1], yb[j-1]) {
			i--
			j--
		}
		s, t = i+smin, j+tmin

		longest = max(longest, s0-s)

		wb[w] = s

		if check && fmin <= k && k <= fmax && s <= wf[w] {
			return s, s0, t, t0, longest, true
		}
	}
	return 0, 0, 0, 0, longest, false
}
