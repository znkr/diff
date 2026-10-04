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
// The results are the following slices:
//   - x0:     x[smin:smax] in as IDs except for elements that appear only in x
//   - y0:     y[tmin:tmax] in as IDs except for elements that appear only in y
//   - xidx:   A mapping from x0 to x: x0[s] corresponds to x[xidx[s]]
//   - yidx:   A mapping from y0 to y: y0[t] corresponds to y[yidx[t]]
//   - counts: The number of times a ID appears in x and y.
//
// Note: The code below is trading some density of the ID space (and with that
// memory) for improved runtime. The bottleneck here are map lookups, the code
// below is structured so that the number of map lookups is minimal.
func preprocess[T comparable](rx, ry []bool, smin, smax, tmin, tmax int, x, y []T) (x0, y0 []int, xidx, yidx []int, counts []int, nanchors int) {
	idx := make(map[T]int, smax-smin) // temporary map from element to ID
	buf := make([]int, 2*(smax-smin)+2*(tmax-tmin))
	x0, buf = buf[:0:smax-smin], buf[smax-smin:]
	xidx, buf = buf[:0:smax-smin], buf[smax-smin:]
	y0, buf = buf[:0:tmax-tmin], buf[tmax-tmin:]
	yidx, buf = buf[:0:tmax-tmin], buf[tmax-tmin:]
	if len(buf) != 0 && cap(buf) != 0 {
		panic("something went wrong during buffer assignments")
	}
	counts = make([]int, smax-smin)
	// Step 1: Create an ID for every element in x[smin:smax] and count the
	// number of occurrences.
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
	// Step 2: Do the same for y, but already ignore everything that's not in x,
	// except for marking these elements as insertions.
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
	return
}
