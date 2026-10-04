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
	"math/rand/v2"
	"testing"
)

func TestSegments(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	for iter := range 4000 {
		// IDs below 8 are anchors if they occur once in x and once in y.
		gen := func() []int {
			x := make([]int, rng.IntN(40))
			for i := range x {
				x[i] = rng.IntN(12)
			}
			return x
		}
		x, y := gen(), gen()
		// counts follows the encoding of preprocess: occurrences in x count
		// 1 up to 2, occurrences in y count 4 up to 8.
		counts := make([]int, 12)
		for _, e := range x {
			counts[e] = min(counts[e]+1, 2)
		}
		for _, e := range y {
			if counts[e] > 0 && counts[e] < 8 {
				counts[e] += 4
			}
		}
		nanchors := 0
		for _, c := range counts {
			if c == 1+4 {
				nanchors++
			}
		}

		// Callers strip the common prefix and suffix of x and y. segments
		// also works for other ranges, in which an anchor's match can be
		// outside the range of the other input.
		smin, smax, tmin, tmax := findChangeBounds(x, y)
		if iter%2 == 1 {
			smin = rng.IntN(len(x) + 1)
			smax = smin + rng.IntN(len(x)-smin+1)
			tmin = rng.IntN(len(y) + 1)
			tmax = tmin + rng.IntN(len(y)-tmin+1)
		}
		got := segments(smin, smax, tmin, tmax, nanchors, counts, x, y)

		if first, last := got[0], got[len(got)-1]; first != (pair{smin, tmin}) || last != (pair{smax, tmax}) {
			t.Fatalf("segments(%v, %v) = %v, want sentinels {%d %d} and {%d %d}", x, y, got, smin, tmin, smax, tmax)
		}
		anchors := got[1 : len(got)-1]
		for i, p := range anchors {
			if p.s < smin || p.s >= smax || p.t < tmin || p.t >= tmax || x[p.s] != y[p.t] || counts[x[p.s]] != 1+4 {
				t.Fatalf("segments(%v, %v) = %v: pair %d isn't an anchor", x, y, got, i+1)
			}
			if i > 0 && (p.s <= anchors[i-1].s || p.t <= anchors[i-1].t) {
				t.Fatalf("segments(%v, %v) = %v isn't increasing", x, y, got)
			}
		}
		if want := longestAnchorSequence(x[:smax], y[:tmax], smin, tmin, counts); len(anchors) != want {
			t.Fatalf("segments(%v, %v) = %v has %d anchors, want %d", x, y, got, len(got)-2, want)
		}
	}
}

// longestAnchorSequence returns the length of the longest sequence of anchors
// in x[smin:] and y[tmin:] that is increasing in x and y.
func longestAnchorSequence(x, y []int, smin, tmin int, counts []int) int {
	var ps []pair
	for s := smin; s < len(x); s++ {
		if counts[x[s]] != 1+4 {
			continue
		}
		for t := tmin; t < len(y); t++ {
			if y[t] == x[s] {
				ps = append(ps, pair{s, t})
			}
		}
	}
	best := 0
	l := make([]int, len(ps))
	for i := range ps {
		l[i] = 1
		for j := range i {
			if ps[j].s < ps[i].s && ps[j].t < ps[i].t {
				l[i] = max(l[i], l[j]+1)
			}
		}
		best = max(best, l[i])
	}
	return best
}
