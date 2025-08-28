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

// Package rvecs contains functions to work with the result vectors, the internal representation
// that's used by the myers algorithm and is then translated to a user facing API. The internal
// representation is separate from the exported representation because it needs to solve a number of
// different problems.
package rvecs

func Make[T any](x, y []T) (rx, ry []bool) {
	r := make([]bool, (len(x) + len(y) + 2))
	rx = r[: len(x)+1 : len(x)+1]
	ry = r[len(x)+1:]
	return
}
