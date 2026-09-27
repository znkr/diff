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

// Package pool provides pools for scratch slices.
package pool

import "sync"

// Slice is a pool of slices with element type T. The zero value is ready to
// use.
type Slice[T any] struct {
	p sync.Pool
}

// Get returns a slice of length n with all elements set to the zero value of T.
func (s *Slice[T]) Get(n int) []T {
	if b, _ := s.p.Get().(*[]T); b != nil && cap(*b) >= n {
		v := (*b)[:n]
		clear(v)
		return v
	}
	return make([]T, n)
}

// GetUncleared returns a slice of length n. Its elements have unspecified
// values.
func (s *Slice[T]) GetUncleared(n int) []T {
	if b, _ := s.p.Get().(*[]T); b != nil && cap(*b) >= n {
		return (*b)[:n]
	}
	return make([]T, n)
}

// Put adds v to the pool. The caller must not use v after Put.
func (s *Slice[T]) Put(v []T) {
	if cap(v) == 0 {
		return
	}
	s.p.Put(&v)
}

var (
	Ints    Slice[int]    // Pool for []int.
	Uint64s Slice[uint64] // Pool for []uint64.
)
