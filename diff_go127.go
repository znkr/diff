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

package diff

import (
	"hash/maphash"

	"znkr.io/diff/internal/config"
	"znkr.io/diff/internal/impl"
	"znkr.io/diff/internal/rvecs"
)

// HunksHash compares the contents of x and y using h to compare and hash
// elements and returns the changes necessary to convert from one to the other.
//
// The output is a sequence of hunks that each describe a number of consecutive
// edits. Hunks include a number of matching elements before and after the last
// delete or insert operation. The number of elements can be configured using
// [Context].
//
// If x and y are identical, the output has length zero.
//
// The following options are supported: [Context], [Minimal], [Fast]
//
// HunksHash uses the algorithm of [Hunks], but it hashes and compares elements
// through h, which makes it about 25% slower than [Hunks]. Use [Hunks] for a
// comparable T whose elements are equal if they are ==. Use HunksHash instead
// of [HunksFunc] if the elements can be hashed.
//
// HunksHash requires Go 1.27.
//
// Important: The output is not guaranteed to be stable and may change with
// minor version upgrades. DO NOT rely on the output being stable.
func HunksHash[T any](x, y []T, h maphash.Hasher[T], opts ...Option) []Hunk[T] {
	cfg := config.FromOptions(opts, config.Context|config.Minimal|config.Fast)
	rx, ry := impl.DiffHash(x, y, h, cfg)
	defer rvecs.Release(rx, ry)
	return hunks(x, y, rx, ry, cfg)
}

// EditsHash compares the contents of x and y using h to compare and hash
// elements and returns the changes necessary to convert from one to the other.
//
// EditsHash returns edits for every element in the input. If both x and y are
// identical, the output will consist of a match edit for every input element.
//
// The following options are supported: [Minimal], [Fast]
//
// EditsHash uses the algorithm of [Edits], but it hashes and compares elements
// through h, which makes it about 25% slower than [Edits]. Use [Edits] for a
// comparable T whose elements are equal if they are ==. Use EditsHash instead
// of [EditsFunc] if the elements can be hashed.
//
// EditsHash requires Go 1.27.
//
// Important: The output is not guaranteed to be stable and may change with
// minor version upgrades. DO NOT rely on the output being stable.
func EditsHash[T any](x, y []T, h maphash.Hasher[T], opts ...Option) []Edit[T] {
	cfg := config.FromOptions(opts, config.Minimal|config.Fast)
	rx, ry := impl.DiffHash(x, y, h, cfg)
	defer rvecs.Release(rx, ry)
	return edits(x, y, rx, ry)
}
