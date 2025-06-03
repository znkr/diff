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

func ExampleIndentHeuristic() {
	x := `// ...
["foo", "bar", "baz"].map do |i|
  i.upcase
end
`

	y := `// ...
["foo", "bar", "baz"].map do |i|
  i
end

["foo", "bar", "baz"].map do |i|
  i.upcase
end
`

	fmt.Println("With textdiff.IndentHeuristic:")
	fmt.Print(textdiff.Unified(x, y, textdiff.IndentHeuristic()))
	fmt.Println()
	fmt.Println("Without textdiff.IndentHeuristic:")
	fmt.Print(textdiff.Unified(x, y))
	// Output:
	// With textdiff.IndentHeuristic:
	// @@ -1,4 +1,8 @@
	//  // ...
	// +["foo", "bar", "baz"].map do |i|
	// +  i
	// +end
	// +
	//  ["foo", "bar", "baz"].map do |i|
	//    i.upcase
	//  end
	//
	// Without textdiff.IndentHeuristic:
	// @@ -1,4 +1,8 @@
	//  // ...
	//  ["foo", "bar", "baz"].map do |i|
	// +  i
	// +end
	// +
	// +["foo", "bar", "baz"].map do |i|
	//    i.upcase
	//  end
}
