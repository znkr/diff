// Copyright 2026 Florian Zenker (flo@znkr.io)
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

//go:build go1.27

package impl

import (
	"hash/maphash"
)

// insertHash is [insert] for elements that h compares.
func insertHash[T any](tab *idTable, x []T, i int, hv uint64, h maphash.Hasher[T]) int {
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

// lookupHash is [lookup] for elements that h compares.
func lookupHash[T any](tab *idTable, x []T, e T, hv uint64, h maphash.Hasher[T]) int {
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
