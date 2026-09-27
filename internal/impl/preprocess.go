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
	"hash/maphash"

	"znkr.io/diff/internal/lines"
	"znkr.io/diff/internal/pool"
	"znkr.io/diff/internal/rvecs"
)

// preprocess performs an important optimization that significantly reduces the
// problem size and time complexity.
//
// For performance reasons, it's doing a number of things at once. This makes it
// quite hard to follow. To understand it, it's necessary to understand the
// individual tasks:
//
// Assign a unique ID to every unique input element in x and y that appears in
// both x and y. This allows us to apply Myers' algorithm on integers instead of
// T (for faster comparison and specialized implementation) and provides a dense
// ID space that makes it possible to use a slice instead of a map to
// efficiently determine which elements exist in both x and y.
//
// Drop all elements that only appear in x or y. These are always deletions and
// insertions respectively. This optimization dramatically reduces the time it
// takes to compute very large diffs, because in practice those diffs will have
// many lines unique to x or y.
//
// Find all anchors, that is all elements that appear exactly once in
// interesting part of x and y (x[smin:smax], y[tmin:tmax]). We do that by
// counting the number of occurrences as 0, 1, many for both x and y. Using 0,
// 1, 2 for counts of elements in x and 0, 4, 8 for counts of elements in y. For
// elements in y, we only count elements that were already found in x. With
// that, a count > 4 means the element appears in both x and y and a count = 1+4
// means the element is an anchor.
//
// The results are in the fields of [preprocessed]. Call [preprocessed.release]
// when they are no longer needed.
//
// Note: The code below is trading some density of the ID space (and with that
// memory) for improved runtime. The bottleneck here are hash table lookups, the
// code below is structured so that the number of lookups is minimal.
func preprocess[T comparable](rx, ry rvecs.Vec, smin, smax, tmin, tmax int, x, y []T) preprocessed {
	p := newPreprocessed(smax-smin, tmax-tmin)

	seed := maphash.MakeSeed()
	tab := newIDTable(smax - smin)
	defer tab.release()

	// Step 1: Create an ID for every element in x[smin:smax] and count the
	// number of occurrences.
	for i, e := range x[smin:smax] {
		id := tab.insert(x, smin+i, maphash.Comparable(seed, e))
		p.addX(id)
	}
	// Step 2: Do the same for y, but already ignore everything that's not in x,
	// except for marking these elements as insertions.
	for i, e := range y[tmin:tmax] {
		id := tab.lookup(x, e, maphash.Comparable(seed, e))
		if id < 0 {
			// Not in x, this is always an insertion.
			ry.Set(i + tmin)
			continue
		}
		p.addY(id, i+tmin)
	}
	// Step 3: Filter out elements from x0 that are not in y.
	p.filterX(rx, smin)
	return p
}

// preprocessLines is [preprocess] for the lines of x and y.
func preprocessLines(rx, ry rvecs.Vec, smin, smax, tmin, tmax int, x, y *lines.Lines) preprocessed {
	p := newPreprocessed(smax-smin, tmax-tmin)

	seed := maphash.MakeSeed()
	tab := newIDTable(smax - smin)
	defer tab.release()

	// Lines between the common prefix and suffix are always split, so the loops
	// read them with At, which inlines.
	for i := smin; i < smax; i++ {
		e := x.At(i)
		id := tab.insertLine(x, i, e, maphash.String(seed, e))
		p.addX(id)
	}
	for i := tmin; i < tmax; i++ {
		e := y.At(i)
		id := tab.lookupLine(x, e, maphash.String(seed, e))
		if id < 0 {
			ry.Set(i)
			continue
		}
		p.addY(id, i)
	}
	p.filterX(rx, smin)
	return p
}

// preprocessed is the result of [preprocess] and [preprocessLines].
type preprocessed struct {
	// x[smin:smax] as IDs except for elements that appear only in x
	x0 []int
	// y[tmin:tmax] as IDs except for elements that appear only in y
	y0 []int
	// x0[s] corresponds to x[xidx[s]]
	xidx []int
	// y0[t] corresponds to y[yidx[t]]
	yidx []int
	// number of times an ID appears in x and y, see [preprocess]
	counts []int

	nanchors int // number of elements that appear exactly once in x and in y

	scratch []int // backs x0, y0, xidx, yidx, and counts
}

// newPreprocessed returns a preprocessed with empty x0 and xidx with capacity
// n, empty y0 and yidx with capacity m, and counts with n zeros. Call
// [preprocessed.release] when it's no longer needed.
func newPreprocessed(n, m int) preprocessed {
	var p preprocessed
	p.scratch = pool.Ints.GetUncleared(3*n + 2*m)
	b := p.scratch
	p.x0, b = b[:0:n], b[n:]
	p.xidx, b = b[:0:n], b[n:]
	p.y0, b = b[:0:m], b[m:]
	p.yidx, b = b[:0:m], b[m:]
	p.counts = b[:n:n]
	clear(p.counts)
	return p
}

// release returns the memory of p to [pool.Ints]. p must not be used after
// release.
func (p *preprocessed) release() { pool.Ints.Put(p.scratch) }

// addX appends the ID of the next element of x to x0 and counts it.
func (p *preprocessed) addX(id int) {
	if c := p.counts[id]; c < 2 {
		p.counts[id] = c + 1
	}
	p.x0 = append(p.x0, id)
}

// addY appends the ID of y[j], which also appears in x, to y0 and j to yidx,
// and counts it.
func (p *preprocessed) addY(id, j int) {
	if c := p.counts[id]; c < 8 {
		p.counts[id] = c + 4
	}
	p.yidx = append(p.yidx, j)
	p.y0 = append(p.y0, id)
}

// filterX removes the elements from x0 that don't appear in y, marks them as
// deletions in rx, and appends the index in x of each remaining element to
// xidx. It sets nanchors.
func (p *preprocessed) filterX(rx rvecs.Vec, smin int) {
	i := 0
	for j, e := range p.x0 {
		if c := p.counts[e]; c > 4 {
			p.xidx = append(p.xidx, j+smin)
			p.x0[i] = e
			if c == 1+4 {
				// Element appears exactly once in x (1) and y (4).
				p.nanchors++
			}
			i++
		} else {
			rx.Set(j + smin) // always an deletion
		}
	}
	p.x0 = p.x0[:i]
}
