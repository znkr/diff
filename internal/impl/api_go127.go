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

	"znkr.io/diff/internal/config"
	"znkr.io/diff/internal/rvecs"
)

// DiffHash compares the contents of x and y with h and returns the changes
// necessary to convert from one to the other.
func DiffHash[T any](x, y []T, h maphash.Hasher[T], cfg config.Config) (rx, ry rvecs.Vec) {
	rx, ry = rvecs.Make(len(x), len(y))

	smin, smax, tmin, tmax := findChangeBoundsHash(x, y, h)
	if handleTrivialBounds(rx, ry, smin, smax, tmin, tmax) {
		return
	}

	p := preprocessHash(rx, ry, smin, smax, tmin, tmax, x, y, h)
	run(rx, ry, &p, cfg)
	p.release()
	return rx, ry
}

// findChangeBoundsHash returns the upper and lower bounds for the changed
// portion of the inputs, comparing elements with h.
func findChangeBoundsHash[T any](x, y []T, h maphash.Hasher[T]) (smin, smax, tmin, tmax int) {
	smin, tmin = 0, 0
	smax, tmax = len(x), len(y)

	// Strip common prefix.
	for smin < smax && tmin < tmax && h.Equal(x[smin], y[tmin]) {
		smin++
		tmin++
	}

	// Strip common suffix.
	for smax > smin && tmax > tmin && h.Equal(x[smax-1], y[tmax-1]) {
		smax--
		tmax--
	}

	return
}
