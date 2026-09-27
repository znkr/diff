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
	"slices"

	"znkr.io/diff/internal/pool"
)

type pair struct{ s, t int }

// segments returns the pairs of indexes of the longest common subsequence of
// anchors in x and y.
//
// The longest common subsequence algorithm is as described in Thomas G.
// Szymanski, “A Special Case of the Maximal Common Subsequence Problem,”
// Princeton TR #170 (January 1975), available at
// https://research.swtch.com/tgs170.pdf.
func segments(smin, smax, tmin, tmax int, nanchors int, counts []int, x, y []int) []pair {
	scratch := pool.Ints.GetUncleared(5*nanchors + len(counts))
	defer pool.Ints.Put(scratch)
	buf := scratch
	var xi, yi, inv, T, L, idx []int
	xi, buf = buf[:0:nanchors], buf[nanchors:]
	yi, buf = buf[:0:nanchors], buf[nanchors:]
	inv, buf = buf[:0:nanchors], buf[nanchors:]
	T, buf = buf[:nanchors], buf[nanchors:]
	L, buf = buf[:nanchors], buf[nanchors:]
	// idx maps an anchor's ID to its index in yi. IDs are dense, so a slice
	// works.
	idx = buf[:len(counts)]

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
	T, L = T[:n], L[:n]
	for i := range T {
		T[i] = n + 1
	}
	for i := range n {
		k, _ := slices.BinarySearch(T, J[i])
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
