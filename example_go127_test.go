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

package diff_test

import (
	"fmt"
	"hash/maphash"
	"strings"

	"znkr.io/diff"
)

// caseInsensitive is a maphash.Hasher[string] whose equivalence relation
// ignores letter case.
type caseInsensitive struct{}

func (caseInsensitive) Hash(h *maphash.Hash, s string) { h.WriteString(strings.ToLower(s)) }

func (caseInsensitive) Equal(a, b string) bool { return strings.ToLower(a) == strings.ToLower(b) }

// Compare two slices of words, ignoring letter case.
func ExampleEditsHash() {
	x := []string{"Hello", "World", "and", "goodbye"}
	y := []string{"hello", "world", "or", "Goodbye"}
	for _, e := range diff.EditsHash(x, y, caseInsensitive{}) {
		switch e.Op {
		case diff.Match:
			fmt.Printf(" %s\n", e.X)
		case diff.Delete:
			fmt.Printf("-%s\n", e.X)
		case diff.Insert:
			fmt.Printf("+%s\n", e.Y)
		}
	}
	// Output:
	//  Hello
	//  World
	// -and
	// +or
	//  goodbye
}
