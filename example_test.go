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

package diff_test

import (
	"fmt"
	"strings"

	"znkr.io/diff"
)

// Compare to strings line by line and output the difference as a pseudo-unified diff output
// (i.e. it's similar to what diff -u would produce). The format is not a correct unified diff
// though, in particular line endings (esp. at the end of the input) are handled differently.
func ExampleHunks_psudoUnified() {
	x := `this paragraph
is not
changed and
barely long
enough to
create a
new hunk

this paragraph
is going to be
removed`

	y := `this is a new paragraph
that is inserted at the top

this paragraph
is not
changed and
barely long
enough to
create a
new hunk`

	xlines := strings.Split(x, "\n")
	ylines := strings.Split(y, "\n")
	hunks := diff.Hunks(xlines, ylines)
	for _, h := range hunks {
		fmt.Printf("@@ -%d,%d +%d,%d @@\n", h.PosX+1, h.EndX-h.PosX, h.PosY+1, h.EndY-h.PosY)
		for _, edit := range h.Edits {
			switch edit.Op {
			case diff.Match:
				fmt.Printf(" %s\n", edit.X)
			case diff.Delete:
				fmt.Printf("-%s\n", edit.X)
			case diff.Insert:
				fmt.Printf("+%s\n", edit.Y)
			default:
				panic("never reached")
			}
		}
	}
	// Output:
	// @@ -1,3 +1,6 @@
	// +this is a new paragraph
	// +that is inserted at the top
	// +
	//  this paragraph
	//  is not
	//  changed and
	// @@ -5,7 +8,3 @@
	//  enough to
	//  create a
	//  new hunk
	// -
	// -this paragraph
	// -is going to be
	// -removed
}

// Compare two strings rune by rune.
func ExampleEdits() {
	x := []rune("Hello, World")
	y := []rune("Hello, 世界")
	edits := diff.Edits(x, y)
	for _, edit := range edits {
		switch edit.Op {
		case diff.Match:
			fmt.Printf("%s", string(edit.X))
		case diff.Delete:
			fmt.Printf("-%s", string(edit.X))
		case diff.Insert:
			fmt.Printf("+%s", string(edit.Y))
		default:
			panic("never reached")
		}
	}
	// Output:
	// Hello, -W-o-r-l-d+世+界
}
