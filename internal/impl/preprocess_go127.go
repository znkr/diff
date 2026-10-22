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

	"znkr.io/diff/internal/rvecs"
)

// preprocessHash is [preprocess] for elements that h compares and hashes.
func preprocessHash[T any](rx, ry rvecs.Vec, smin, smax, tmin, tmax int, x, y []T, h maphash.Hasher[T]) preprocessed {
	p := newPreprocessed(smax-smin, tmax-tmin)

	// The zero Hash has a random seed, and Reset keeps it.
	var mh maphash.Hash
	tab := newIDTable(smax - smin)
	defer tab.release()

	for i, e := range x[smin:smax] {
		mh.Reset()
		h.Hash(&mh, e)
		id := insertHash(tab, x, smin+i, mh.Sum64(), h)
		p.addX(id)
	}
	for i, e := range y[tmin:tmax] {
		mh.Reset()
		h.Hash(&mh, e)
		id := lookupHash(tab, x, e, mh.Sum64(), h)
		if id < 0 {
			ry.Set(i + tmin)
			continue
		}
		p.addY(id, i+tmin)
	}
	p.filterX(rx, smin)
	return p
}
