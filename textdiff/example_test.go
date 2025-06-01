package textdiff_test

import (
	"fmt"

	"znkr.io/diff/textdiff"
)

func ExampleUnified() {
	x := `this paragraph
is not
changed and
barely long
enough to
create a
new hunk

this paragraph
is going to be
removed
`

	y := `this is a new paragraph
that is inserted at the top

this paragraph
is not
changed and
barely long
enough to
create a
new hunk
`
	fmt.Print(textdiff.Unified(x, y))
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
