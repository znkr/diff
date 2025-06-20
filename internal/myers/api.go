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

package myers

import "znkr.io/diff/internal/config"

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
	var m myersInt
	m.xidx, m.yidx = xidx, yidx
	m.rx, m.ry = rx, ry
	smin0, smax0, tmin0, tmax0 := m.init(x0, y0)
	m.compare(smin0, smax0, tmin0, tmax0, cfg.Optimal)

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
