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
	"math/bits"

	"znkr.io/diff/internal/lines"
	"znkr.io/diff/internal/pool"
)

// idTable maps elements to dense IDs in the order they are inserted. The table
// doesn't hold the elements; insert and lookup compare the elements of a slice,
// insertHash and lookupHash do the same with a [maphash.Hasher], and
// insertLine and lookupLine compare lines of an input.
type idTable struct {
	// slots is an open addressing hash table with linear probing. A slot holds
	// the upper 16 bits of the element's hash and ID+1 in the lower 48 bits. An
	// empty slot is 0.
	slots []uint64
	mask  uint64

	// reps maps an ID to the index of the first element in x with that ID.
	reps []int
}

// idMask selects the ID+1 in a slot. IDs are less than the number of elements
// in x, and 48 bits hold any number that preprocessing can allocate memory for:
// it allocates more than 8 bytes per element, and a Go heap has at most 2^48
// bytes.
const idMask = 1<<48 - 1

// newIDTable returns an empty table with room for n elements.
func newIDTable(n int) *idTable {
	// Keep the load factor at or below 1/2.
	size := 1 << bits.Len(uint(2*n))
	return &idTable{
		slots: pool.Uint64s.Get(size),
		mask:  uint64(size - 1),
		reps:  pool.Ints.GetUncleared(n)[:0],
	}
}

// release returns the memory of tab to the pools. tab must not be used after
// release.
func (tab *idTable) release() {
	pool.Uint64s.Put(tab.slots)
	pool.Ints.Put(tab.reps)
}

// add adds an entry with the given tag for element i to slot j and returns its
// ID.
func (tab *idTable) add(j, tag uint64, i int) int {
	id := len(tab.reps)
	tab.reps = append(tab.reps, i)
	tab.slots[j] = tag | uint64(id+1)
	return id
}

// insert returns the ID of x[i], adding it to tab if it's not present. h is the
// hash of x[i].
func (tab *idTable) insert[T comparable](x []T, i int, h uint64) int {
	tag := h &^ idMask
	for j := h & tab.mask; ; j = (j + 1) & tab.mask {
		slot := tab.slots[j]
		if slot == 0 {
			return tab.add(j, tag, i)
		}
		if slot&^idMask == tag {
			// The tags match, but only comparing the elements rules out a
			// collision.
			id := int(slot&idMask) - 1
			if x[tab.reps[id]] == x[i] {
				return id
			}
		}
	}
}

// lookup returns the ID of e or -1 if it's not in tab. h is the hash of e.
func (tab *idTable) lookup[T comparable](x []T, e T, h uint64) int {
	tag := h &^ idMask
	for j := h & tab.mask; ; j = (j + 1) & tab.mask {
		slot := tab.slots[j]
		if slot == 0 {
			return -1
		}
		if slot&^idMask == tag {
			id := int(slot&idMask) - 1
			if x[tab.reps[id]] == e {
				return id
			}
		}
	}
}

// insertHash is [idTable.insert] for elements that h compares.
func (tab *idTable) insertHash[T any](x []T, i int, hv uint64, h maphash.Hasher[T]) int {
	tag := hv &^ idMask
	for j := hv & tab.mask; ; j = (j + 1) & tab.mask {
		slot := tab.slots[j]
		if slot == 0 {
			return tab.add(j, tag, i)
		}
		if slot&^idMask == tag {
			id := int(slot&idMask) - 1
			if h.Equal(x[tab.reps[id]], x[i]) {
				return id
			}
		}
	}
}

// lookupHash is [idTable.lookup] for elements that h compares.
func (tab *idTable) lookupHash[T any](x []T, e T, hv uint64, h maphash.Hasher[T]) int {
	tag := hv &^ idMask
	for j := hv & tab.mask; ; j = (j + 1) & tab.mask {
		slot := tab.slots[j]
		if slot == 0 {
			return -1
		}
		if slot&^idMask == tag {
			id := int(slot&idMask) - 1
			if h.Equal(x[tab.reps[id]], e) {
				return id
			}
		}
	}
}

// insertLine returns the ID of line i of x, adding it to tab if it's not
// present. e is the line and h is its hash.
func (tab *idTable) insertLine(x *lines.Lines, i int, e string, h uint64) int {
	tag := h &^ idMask
	for j := h & tab.mask; ; j = (j + 1) & tab.mask {
		slot := tab.slots[j]
		if slot == 0 {
			return tab.add(j, tag, i)
		}
		if slot&^idMask == tag {
			id := int(slot&idMask) - 1
			if x.At(tab.reps[id]) == e {
				return id
			}
		}
	}
}

// lookupLine returns the ID of line e or -1 if it's not in tab. x holds the
// lines tab was built from. h is the hash of e.
func (tab *idTable) lookupLine(x *lines.Lines, e string, h uint64) int {
	tag := h &^ idMask
	for j := h & tab.mask; ; j = (j + 1) & tab.mask {
		slot := tab.slots[j]
		if slot == 0 {
			return -1
		}
		if slot&^idMask == tag {
			id := int(slot&idMask) - 1
			if x.At(tab.reps[id]) == e {
				return id
			}
		}
	}
}
