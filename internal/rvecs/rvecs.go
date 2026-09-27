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

// Package rvecs contains functions to work with the result vectors, the
// internal representation that's used by the myers algorithm and is then
// translated to a user facing API. The internal representation is separate from
// the exported representation because it needs to solve a number of different
// problems.
package rvecs

import (
	"math/bits"

	"znkr.io/diff/internal/pool"
)

// Vec is a result vector: a bit vector with one bit per input element. A set
// bit marks a changed element. The zero value has length 0.
type Vec struct {
	w []uint64
	n int
}

// Make returns zeroed result vectors for inputs with n and m elements. Each
// vector has one extra element at the end that stays unchanged. Call [Release]
// when they are no longer needed.
func Make(n, m int) (rx, ry Vec) {
	nx, ny := words(n+1), words(m+1)
	w := pool.Uint64s.Get(nx + ny)
	rx = Vec{w: w[:nx], n: n + 1}
	ry = Vec{w: w[nx:], n: m + 1}
	return
}

// Release returns result vectors created by [Make] to the pool. The caller must
// not use rx or ry after Release.
func Release(rx, ry Vec) {
	// Make allocates rx and ry as one slice with ry directly after rx, and rx.w
	// has the capacity of the whole slice.
	pool.Uint64s.Put(rx.w[:cap(rx.w)])
}

func words(n int) int { return (n + 63) / 64 }

// Len returns the number of elements in v.
func (v Vec) Len() int { return v.n }

// Get reports whether element i is changed.
func (v Vec) Get(i int) bool { return v.w[uint(i)/64]&(1<<(uint(i)%64)) != 0 }

// Set marks element i as changed.
func (v Vec) Set(i int) { v.w[uint(i)/64] |= 1 << (uint(i) % 64) }

// Clear marks element i as unchanged.
func (v Vec) Clear(i int) { v.w[uint(i)/64] &^= 1 << (uint(i) % 64) }

// SetRange marks elements [i, j) as changed.
func (v Vec) SetRange(i, j int) {
	if i >= j {
		return
	}
	lo, hi := uint(i)/64, uint(j-1)/64
	loMask := ^uint64(0) << (uint(i) % 64)
	hiMask := ^uint64(0) >> (63 - uint(j-1)%64)
	if lo == hi {
		v.w[lo] |= loMask & hiMask
		return
	}
	v.w[lo] |= loMask
	for k := lo + 1; k < hi; k++ {
		v.w[k] = ^uint64(0)
	}
	v.w[hi] |= hiMask
}

// SetSorted marks the elements with the indices in idx as changed. idx must be
// strictly increasing.
func (v Vec) SetSorted(idx []int) {
	if len(idx) == 0 {
		return
	}
	if first, last := idx[0], idx[len(idx)-1]; last-first == len(idx)-1 {
		// A strictly increasing idx with this span holds every index in [first,
		// last].
		v.SetRange(first, last+1)
		return
	}
	for _, i := range idx {
		v.Set(i)
	}
}

// NextSet returns the smallest j >= i with element j changed, or Len() if there
// is none.
func (v Vec) NextSet(i int) int {
	if i >= v.n {
		return v.n
	}
	// Bits at and after n are never set.
	k := uint(i) / 64
	if w := v.w[k] >> (uint(i) % 64); w != 0 {
		return i + bits.TrailingZeros64(w)
	}
	for k++; k < uint(len(v.w)); k++ {
		if w := v.w[k]; w != 0 {
			return int(k)*64 + bits.TrailingZeros64(w)
		}
	}
	return v.n
}

// NextClear returns the smallest j >= i with element j unchanged, or Len() if
// there is none.
func (v Vec) NextClear(i int) int {
	if i >= v.n {
		return v.n
	}
	k := uint(i) / 64
	if w := ^v.w[k] >> (uint(i) % 64); w != 0 {
		return min(i+bits.TrailingZeros64(w), v.n)
	}
	for k++; k < uint(len(v.w)); k++ {
		if w := ^v.w[k]; w != 0 {
			return min(int(k)*64+bits.TrailingZeros64(w), v.n)
		}
	}
	return v.n
}

// PrevSet returns the largest j <= i with element j changed, or -1 if there is
// none. i must be less than Len().
func (v Vec) PrevSet(i int) int {
	if i < 0 {
		return -1
	}
	k := uint(i) / 64
	// Shift out the bits after i.
	if w := v.w[k] << (63 - uint(i)%64); w != 0 {
		return i - bits.LeadingZeros64(w)
	}
	for k > 0 {
		k--
		if w := v.w[k]; w != 0 {
			return int(k)*64 + 63 - bits.LeadingZeros64(w)
		}
	}
	return -1
}

// PrevClear returns the largest j <= i with element j unchanged, or -1 if there
// is none. i must be less than Len().
func (v Vec) PrevClear(i int) int {
	if i < 0 {
		return -1
	}
	k := uint(i) / 64
	if w := ^v.w[k] << (63 - uint(i)%64); w != 0 {
		return i - bits.LeadingZeros64(w)
	}
	for k > 0 {
		k--
		if w := ^v.w[k]; w != 0 {
			return int(k)*64 + 63 - bits.LeadingZeros64(w)
		}
	}
	return -1
}
