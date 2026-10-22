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
	"fmt"

	"znkr.io/diff/internal/config"
	"znkr.io/diff/internal/lines"
	"znkr.io/diff/internal/rvecs"
)

// Diff compares the contents of x and y and returns the changes necessary to
// convert from one to the other.
func Diff[T comparable](x, y []T, cfg config.Config) (rx, ry rvecs.Vec) {
	rx, ry = rvecs.Make(len(x), len(y))

	smin, smax, tmin, tmax := findChangeBounds(x, y)
	if handleTrivialBounds(rx, ry, smin, smax, tmin, tmax) {
		return
	}

	// Preprocess x and y to reduce the problem size and to work with integer
	// IDs instead of Ts. This is only possible for comparable types, because
	// assigning IDs hashes and compares Ts.
	p := preprocess(rx, ry, smin, smax, tmin, tmax, x, y)
	run(rx, ry, &p, cfg)
	p.release()
	return rx, ry
}

// DiffLines compares the lines of x and y and returns the changes necessary to
// convert from one to the other. prefix and suffix are the number of lines in
// the longest common prefix and suffix of x and y, as returned by
// [lines.SplitPair]; DiffLines doesn't compare them.
func DiffLines(x, y *lines.Lines, prefix, suffix int, cfg config.Config) (rx, ry rvecs.Vec) {
	n, m := x.Len(), y.Len()
	rx, ry = rvecs.Make(n, m)

	smin, smax, tmin, tmax := prefix, n-suffix, prefix, m-suffix
	if handleTrivialBounds(rx, ry, smin, smax, tmin, tmax) {
		return
	}

	p := preprocessLines(rx, ry, smin, smax, tmin, tmax, x, y)
	run(rx, ry, &p, cfg)
	p.release()
	return rx, ry
}

// run computes the diff of the preprocessed inputs in p in the mode given by
// cfg and records the result in rx and ry.
func run(rx, ry rvecs.Vec, p *preprocessed, cfg config.Config) {
	switch cfg.Mode {
	case config.ModeMinimal:
		diffMinimal(rx, ry, p.x0, p.y0, p.xidx, p.yidx)

	case config.ModeDefault:
		diffDefault(rx, ry, p.x0, p.y0, p.xidx, p.yidx, p.counts, p.nanchors, cfg.ForceAnchoringHeuristic)

	case config.ModeFast:
		diffFast(rx, ry, p.x0, p.y0, p.xidx, p.yidx, p.counts, p.nanchors)

	default:
		panic(fmt.Sprintf("unknown mode: %v", cfg.Mode))
	}
}

// DiffFunc compares the contents of x and y and returns the changes necessary
// to convert from one to the other.
//
// Note that this function has generally worse performance than [Diff] for diffs
// with many changes.
func DiffFunc[T any](x, y []T, eq func(a, b T) bool, cfg config.Config) (rx, ry rvecs.Vec) {
	var m myers[T]
	m.rx, m.ry = rvecs.Make(len(x), len(y))
	// init strips the common prefix and suffix, and compare handles an empty x
	// or y.
	smin, smax, tmin, tmax := m.init(x, y, eq)
	m.compare(smin, smax, tmin, tmax, cfg.Mode == config.ModeMinimal, eq)
	m.release()
	return m.rx, m.ry
}

// findChangeBounds returns the upper and lower bounds for the changed portion
// of the inputs.
func findChangeBounds[T comparable](x, y []T) (smin, smax, tmin, tmax int) {
	smin, tmin = 0, 0
	smax, tmax = len(x), len(y)

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

	return
}

// handleTrivialBounds handles trivial bounds. It returns true if the bounds are
// trivial.
func handleTrivialBounds(rx, ry rvecs.Vec, smin, smax, tmin, tmax int) bool {
	switch {
	case smin != smax && tmin == tmax:
		rx.SetRange(smin, smax)
		return true
	case smin == smax && tmin != tmax:
		ry.SetRange(tmin, tmax)
		return true
	case smin == smax && tmin == tmax:
		return true
	default:
		return false
	}
}

func diffMinimal(rx, ry rvecs.Vec, x0, y0 []int, xidx, yidx []int) {
	var m myersInt
	m.xidx, m.yidx = xidx, yidx
	m.rx, m.ry = rx, ry
	smin0, smax0, tmin0, tmax0 := m.init(x0, y0)
	m.compare(smin0, smax0, tmin0, tmax0, true)
	m.release()
}

func diffDefault(rx, ry rvecs.Vec, x0, y0 []int, xidx, yidx []int, counts []int, nanchors int, forceAnchoring bool) {
	var m myersInt
	m.xidx, m.yidx = xidx, yidx
	m.rx, m.ry = rx, ry
	smin0, smax0, tmin0, tmax0 := m.init(x0, y0)
	defer m.release()

	// Heuristic (ANCHORING): If the input is too large and we have found
	// anchors, use the anchoring heuristic. This provides a significant
	// performance boost and provides more optimal results than the other
	// heuristics.
	anchoring := nanchors > 0 && (smax0-smin0)+(tmax0-tmin0) > anchoringHeuristicMinInputLen
	if anchoring || forceAnchoring {
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

			m.compare(done.s, start.s, done.t, start.t, false)

			if end.s >= smax0 && end.t >= tmax0 {
				break
			}

			done = end
		}
	} else {
		m.compare(smin0, smax0, tmin0, tmax0, false)
	}
}

func diffFast(rx, ry rvecs.Vec, x0, y0 []int, xidx, yidx []int, counts []int, nanchors int) {
	// Fast mode uses patience diff.
	smin0, smax0, tmin0, tmax0 := findChangeBounds(x0, y0)
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

		rx.SetSorted(xidx[done.s:start.s])
		ry.SetSorted(yidx[done.t:start.t])

		if end.s >= smax0 && end.t >= tmax0 {
			break
		}

		done = end
	}
}
