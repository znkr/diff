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
// The algorithm was independently discovered by Ekko Ukkonen:
//
// Ukkonen, E. Algorithms for approximate string matching. Information and Control, Volume 64,
// Issues 1-3, 100-118 (1985). https://doi.org/10.1016/S0019-9958(85)80046-2
//
// # Heuristics
//
// ANCHORING: A heuristic used anchor the diff around lines that are provably one 1:1
// correspondences in both files. This heuristic is similar to the patience diff algorithm, but the
// idea is used as a heuristic to reduce the problem size. This heuristic speeds up diffs for large
// files and produces better diffs than other heuristics we use to limit the time complexity of the
// algorithm. However, this heuristic only works for comparable types.
//
// GOOD_DIAGONAL: A heuristic used by many diff implementations to eagerly use a good diagonal as a
// split point instead of trying to find an optimal one.
//
// TOO_EXPENSIVE: A heuristic by Paul Eggert that reduces the time complexity significantly for
// large files with many differences at the cost of suboptimal diffs. If the search for an optimal
// d-path exceeds a cost limit (in terms of d), the search is aborted and the furthest reaching
// d-path that optimizes x + y is used to determine a split.
package myers
