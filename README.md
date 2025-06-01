# diff

A difference algorithm module for Go.

Difference algorithms compare two inputs and find the edits that transform one to the other. This is
very useful to understand changes, for example when comparing a test result with the tests
expectation or to understand which changes have been made to a file.

This module provides diffing for arbitrary Go slices and text.

Documentation at http://pkg.go.dev/znkr.io/diff.

## Stability: WIP

This project is a work in progress, pending reviews and better testing. Feedback is very welcome.

## Example - Comparing Slices

Diffing two slices produces either the full list of edits

```go
x := strings.Fields("calm seas reflect the sky")
y := strings.Fields("restless seas reflect the sky defiantly")
edits := diff.Edits(x, y)
for _, edit := range edits {
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
// Output:
// -calm
// +restless
//
//	seas
//	reflect
//	the
//	sky
//
// +defiantly
```

or a list of hunks representing consecutive edits

```go
x := strings.Fields("calm seas reflect the sky")
y := strings.Fields("restless seas reflect the sky defiantly")
hunks := diff.Hunks(x, y, diff.Context(1))
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
// @@ -1,2 +1,2 @@
// -calm
// +restless
//  seas
// @@ -5,1 +5,2 @@
//  sky
// +defiantly
```

For both functions, a `...Func` variant exists that works with arbitrary slices by taking an
equality function.

## Example - Comparing Text

Because of it's importance, comparing text line by line has special support and produces output
in the unified diff format:

```go
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
```

## License

This module is distributed under the [Apache License, Version
2.0](https://www.apache.org/licenses/LICENSE-2.0), see [LICENSE](LICENSE) for more information.