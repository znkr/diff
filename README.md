# znkr.io/diff

[![Go Reference](https://pkg.go.dev/badge/znkr.io/diff.svg)](https://pkg.go.dev/znkr.io/diff)
[![Go Report Card](https://goreportcard.com/badge/znkr.io/diff)](https://goreportcard.com/report/znkr.io/diff)

A high-performance difference algorithm module for Go.

Difference algorithms compare two inputs and find the edits that transform one to the other. This is
very useful to understand changes, for example when comparing a test result with the expected result
or to understand which changes have been made to a file.

This module provides diffing for arbitrary Go slices and text.

## Installation

To use this module in your Go project, run:

```bash
go get znkr.io/diff
```

## API Documentation

Full documentation available at [pkg.go.dev/znkr.io/diff](https://pkg.go.dev/znkr.io/diff).

## Examples

### Comparing Slices

Diffing two slices produces either the full list of edits

```go
x := strings.Fields("calm seas reflect the sky")
y := strings.Fields("restless seas reflect the sky defiantly")
edits := diff.Edits(x, y)
for i, edit := range edits {
    if i > 0 {
        fmt.Print(" ")
    }
    switch edit.Op {
    case diff.Match:
        fmt.Printf("%s", edit.X)
    case diff.Delete:
        fmt.Printf("[-%s-]", edit.X)
    case diff.Insert:
        fmt.Printf("{+%s+}", edit.Y)
    default:
        panic("never reached")
    }
}
// Output:
// [-calm-] {+restless+} seas reflect the sky {+defiantly+}
```

or a list of hunks representing consecutive edits

```go
x := strings.Fields("calm seas reflect the sky")
y := strings.Fields("restless seas reflect the sky defiantly")
hunks := diff.Hunks(x, y, diff.Context(1))
for i, h := range hunks {
    if i > 0 {
        fmt.Print(" … ")
    }
    for i, edit := range h.Edits {
        if i > 0 {
            fmt.Print(" ")
        }
        switch edit.Op {
        case diff.Match:
            fmt.Printf("%s", edit.X)
        case diff.Delete:
            fmt.Printf("[-%s-]", edit.X)
        case diff.Insert:
            fmt.Printf("{+%s+}", edit.Y)
        default:
            panic("never reached")
        }
    }
}
// Output:
// [-calm-] {+restless+} seas … sky {+defiantly+}
```

For both functions, a `...Func` variant exists that works with arbitrary slices by taking an
equality function.

### Comparing Text

Because of its importance, comparing text line by line has special support and produces output
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

## Stability

**Status: Beta** - This project is in beta, pending API reviews and general feedback, both are very
welcome.

As a general rule, the exact diff output will never be guaranteed to be stable: I expect that
performance and quality improvements will always be possible and they will likely change the output
of a diff. Therefore, committing to a stable diff result would be too limiting.


## Diff Readability

Diffs produced by this module are intended to be readable by humans.

Readable diffs have been the subject of a lot of discussions and have even resulted in some new
diffing algorithms like the patience or histogram algorithms in git. However, the best work about
diff readability by far is [diff-slider-tools](https://github.com/mhagger/diff-slider-tools) by
[Michael Haggerty](https://github.com/mhagger). He implemented a heuristic that's applied in a
post-processing step to improve the readability. This module implements this heuristic in the
[textdiff](https://pkg.go.dev/znkr.io/diff/textdiff) package.

For example:

```go
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
```

## Performance

By default, the underlying diff algorithm used is Myers' algorithm augmented by a number of
heuristics to speed up the algorithm in exchange for non-minimal diffs. The `diff.Optimal` option is
provided to skip these heuristics to get a minimal diff independent of the costs and `diff.Fast` to
use a fast heuristic to get a non-minimal diff as fast as possible.

On an M1 Mac, the default settings almost always result in runtimes &lt; 1 ms, but truly large diffs
(e.g. caused by changing generators for generated files) can result in runtimes of almost 100 ms.
Below is the distribution of runtimes applying `textdiff.Unified` to every commit in the [Go
repository](http://go.googlesource.com/go)  (y-axis is in log scale):

![histogram of textdiff.Unified runtime](plots/perf_go_repo.png)

### Comparison with other Implementations

Comparing the performance with other Go modules that implement the same features is always
interesting, because it can surface missed optimization opportunities. This is especially
interesting for larger inputs where superlinear growth can become a problem. Below are benchmarks of
`znkr.io/diff` against other popular Go diff modules:

- **znkr**: Default configuration with performance optimizations enabled
- **znkr-optimal**: With `diff.Optimal()` option for minimal diffs
- **znkr-fast**: With `diff.Fast()` option for fastest possible diffing
- **go-internal**: Patience diff algorithm from [`github.com/rogpeppe/go-internal`](https://github.com/rogpeppe/go-internal)
- **diffmatchpatch**: Implementation from [`github.com/sergi/go-diff`](https://github.com/sergi/go-diff)
- **godebug**: Implementation from [`golang.org/x/tools/godebug`](https://pkg.go.dev/golang.org/x/tools/godebug)
- **mb0**: Implementation from [`github.com/mb0/diff`](https://github.com/mb0/diff)
- **udiff**: Implementation from [`github.com/aymanbagabas/go-udiff`](https://github.com/aymanbagabas/go-udiff)

**Note:** It's possible that the benchmark is using `diffmatchpatch` incorrectly, the benchmark
numbers certainly look suspiciously high. However, the way it's used in the benchmark is used in
at least one large open source project.

#### Runtime Performance (seconds per operation)

On the benchmarks used for this comparison znkr.io/diff almost always outperforms the other
implementations. However, there's one case where go-internal is significantly faster, but the
resulting diff is 10% larger (see numbers below).

| Test Case | znkr (baseline) | znkr-optimal | znkr-fast | go-internal | diffmatchpatch | godebug | mb0 | udiff |
|-----------|-----------------|--------------|-----------|-------------|----------------|---------|-----|-------|
| **large_01** | 2.707ms | 10.993ms<br>(+306.14%) | 2.642ms<br>(-2.40%) | 4.928ms<br>(+82.04%) | 43.205ms<br>(+1496.15%) | 181.374ms<br>(+6600.66%) | 84.950ms<br>(+3038.39%) | 7.915ms<br>(+192.40%) |
| **large_02** | 20.591ms | 49.798ms<br>(+141.84%) | 1.840ms<br>(-91.06%) | 4.139ms<br>(-79.90%) | 623.986ms<br>(+2930.32%) | 3000.340ms<br>(+14470.84%) | 1513.701ms<br>(+7251.13%) | 6.457ms<br>(-68.64%) |
| **large_03** | 3.210ms | 15.138ms<br>(+371.61%) | 3.130ms<br>(-2.49%) | 4.688ms<br>(+46.04%) | 31.851ms<br>(+892.26%) | 187.093ms<br>(+5728.54%) | 105.379ms<br>(+3182.89%) | 10.057ms<br>(+213.31%) |
| **large_04** | 7.125ms | 249.229ms<br>(+3397.94%) | 5.557ms<br>(-22.01%) | 8.656ms<br>(+21.49%) | 1012.579ms<br>(+14111.61%) | 13230.536ms<br>(+185591.43%) | 2229.906ms<br>(+31196.87%) | 15.818ms<br>(+122.01%) |
| **medium** | 26.79µs | 27.38µs<br>(+2.23%) | 27.54µs<br>(+2.81%) | 64.70µs<br>(+141.55%) | 258.27µs<br>(+864.18%) | 705.62µs<br>(+2534.24%) | 269.56µs<br>(+906.34%) | 290.81µs<br>(+985.66%) |
| **small** | 18.30µs | 18.49µs<br>(+1.05%) | 18.43µs<br>(±0%) | 38.06µs<br>(+107.97%) | 78.23µs<br>(+327.41%) | 200.04µs<br>(+992.97%) | 52.86µs<br>(+188.83%) | 106.99µs<br>(+484.55%) |

#### Diff Minimality (number of edits produced)

| Test Case | znkr (baseline) | znkr-optimal | znkr-fast | go-internal | diffmatchpatch | godebug | mb0 | udiff |
|-----------|----------------|--------------|-----------|-------------|----------------|---------|-----|-------|
| **large_01** | 5.615k edits | 5.615k edits<br>(±0%) | 5.615k edits<br>(±0%) | 5.617k edits<br>(+0.04%) | 5.615k edits<br>(±0%) | 5.615k edits<br>(±0%) | 5.615k edits<br>(±0%) | 35.805k edits<br>(+537.67%) |
| **large_02** | 28.87k edits | 28.83k edits<br>(-0.15%) | 31.80k edits<br>(+10.15%) | 31.81k edits<br>(+10.17%) | 28.83k edits<br>(-0.14%) | 28.83k edits<br>(-0.15%) | 28.83k edits<br>(-0.15%) | 31.80k edits<br>(+10.13%) |
| **large_03** | 5.504k edits | 5.504k edits<br>(±0%) | 5.504k edits<br>(±0%) | 5.506k edits<br>(+0.04%) | 5.504k edits<br>(±0%) | 5.504k edits<br>(±0%) | 5.504k edits<br>(±0%) | 55.738k edits<br>(+912.68%) |
| **large_04** | 26.99k edits | 26.99k edits<br>(-0.01%) | 27.80k edits<br>(+2.99%) | 27.80k edits<br>(+2.99%) | 60.36k edits<br>(+123.65%) | 26.99k edits<br>(-0.01%) | 26.99k edits<br>(-0.01%) | 103.22k edits<br>(+282.45%) |
| **medium** | 277 edits | 277 edits<br>(±0%) | 277 edits<br>(±0%) | 283 edits<br>(+2.17%) | 277 edits<br>(±0%) | 277 edits<br>(±0%) | 277 edits<br>(±0%) | 431 edits<br>(+55.60%) |
| **small** | 108 edits | 108 edits<br>(±0%) | 114 edits<br>(+5.56%) | 120 edits<br>(+11.11%) | 108 edits<br>(±0%) | 108 edits<br>(±0%) | 108 edits<br>(±0%) | 280 edits<br>(+159.26%) |

## Correctness

I tested this diff implementation against every commit in the [Go
repository](http://go.googlesource.com/go) using the standard unix `patch` tool to ensure that all
diff results are correct.

This test is part of the test suite for this module and can be run with

```
go run ./internal/cmd/eval -repo <repo>
```

## License

This module is distributed under the [Apache License, Version
2.0](https://www.apache.org/licenses/LICENSE-2.0), see [LICENSE](LICENSE) for more information.
