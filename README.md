# znkr.io/diff

[![Go Reference](https://pkg.go.dev/badge/znkr.io/diff.svg)](https://pkg.go.dev/znkr.io/diff)

A high-performance difference algorithm module for Go.

Difference algorithms compare two inputs and find the edits that transform one to the other. This is
very useful to understand changes, for example when comparing a test result with the expected result
or to understand which changes have been made to a file.

This module provides diffing for arbitrary Go slices and text.

I wrote a bit about the background and the design decisions that went into this module on
[flo.znkr.io/diff](https://flo.znkr.io/diff).

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
equality function, and a `...Hash` variant that takes a `maphash.Hasher`. Use the `...Hash` variant
instead of the `...Func` variant if the elements can be hashed. It's about 25% slower than `Hunks`
and `Edits`, so use those for comparable types.

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

**API: Stable** - The API is stable and follows the Go module compatibility
[guidelines](https://go.dev/doc/modules/version-policy).

The exact diff output is not guaranteed to be stable: performance and quality improvements will
likely change the output of a diff. Committing to a stable diff result would be too limiting.


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
heuristics to speed up the algorithm in exchange for non-minimal diffs. The `diff.Minimal` option is
provided to skip these heuristics to get a minimal diff independent of the costs and `diff.Fast` to
use a fast heuristic to get a non-minimal diff as fast as possible.

On an M1 Mac, the default settings almost always result in runtimes &lt; 1 ms, but truly large diffs
(e.g. caused by changing generators for generated files) can result in runtimes of about 100 ms.
Below is the distribution of runtimes applying `textdiff.Unified` to every commit in the [Go
repository](http://go.googlesource.com/go)  (y-axis is in log scale). It's produced by `go run
./internal/cmd/eval -repo <repo> -stats stats.csv -validate=false` and the notebook in
[plots](plots/perf_go_repo.ipynb):

![histogram of textdiff.Unified runtime](plots/perf_go_repo.png)

### Comparison with other Implementations

Comparing the performance with other Go modules that implement the same features is always
interesting, because it can surface missed optimization opportunities. This is especially
interesting for larger inputs where superlinear growth can become a problem. Below are benchmarks of
`znkr.io/diff` against other popular Go diff modules:

- **znkr**: Default configuration with performance optimizations enabled
- **znkr-minimal**: With `diff.Minimal()` option for minimal diffs
- **znkr-fast**: With `diff.Fast()` option for fastest possible diffing
- **go-internal**: Patience diff algorithm from [`github.com/rogpeppe/go-internal`](https://github.com/rogpeppe/go-internal)
- **diffmatchpatch**: Implementation from [`github.com/sergi/go-diff`](https://github.com/sergi/go-diff)
- **godebug**: Implementation from [`github.com/kylelemons/godebug`](https://github.com/kylelemons/godebug)
- **mb0**: Implementation from [`github.com/mb0/diff`](https://github.com/mb0/diff)
- **udiff**: Implementation from [`github.com/aymanbagabas/go-udiff`](https://github.com/aymanbagabas/go-udiff)

**Note:** It's possible that the benchmark is using `diffmatchpatch` incorrectly, the benchmark
numbers certainly look suspiciously high. However, the way it's used in the benchmark is used in
at least one large open source project.

#### Runtime Performance (seconds per operation)

On the benchmarks used for this comparison znkr.io/diff almost always outperforms the other
implementations. However, there's one case where go-internal and udiff are significantly faster, but
the resulting diffs are 10% larger (see numbers below). On that case, `diff.Fast` is faster than
both and produces diffs of the same size. The numbers are from an M1 Pro, produced with the command
in [internal/benchmarks](internal/benchmarks/README.md).

| Test Case | znkr (baseline) | znkr-minimal | znkr-fast | go-internal | diffmatchpatch | godebug | mb0 | udiff |
|-----------|-----------------|--------------|-----------|-------------|----------------|---------|-----|-------|
| **large_01** | 1.248ms | 8.214ms<br>(+558.06%) | 1.254ms<br>(±0%) | 4.403ms<br>(+252.75%) | 43.629ms<br>(+3395.29%) | 1296.260ms<br>(+103747.24%) | 81.315ms<br>(+6414.37%) | 6.746ms<br>(+440.44%) |
| **large_02** | 18.609ms | 42.815ms<br>(+130.08%) | 1.078ms<br>(-94.21%) | 3.984ms<br>(-78.59%) | 627.153ms<br>(+3270.16%) | 6147.497ms<br>(+32935.03%) | 1420.640ms<br>(+7534.15%) | 6.814ms<br>(-63.38%) |
| **large_03** | 1.859ms | 11.836ms<br>(+536.58%) | 1.845ms<br>(±0%) | 4.201ms<br>(+125.96%) | 31.971ms<br>(+1619.53%) | 1555.402ms<br>(+83554.82%) | 97.524ms<br>(+5145.16%) | 11.104ms<br>(+497.19%) |
| **large_04** | 4.804ms | 221.140ms<br>(+4503.06%) | 3.394ms<br>(-29.34%) | 7.636ms<br>(+58.95%) | 1016.767ms<br>(+21064.14%) | 10813.805ms<br>(+224990.69%) | 2109.423ms<br>(+43807.90%) | 19.696ms<br>(+309.97%) |
| **medium** | 24.67µs | 24.96µs<br>(±0%) | 24.87µs<br>(±0%) | 70.98µs<br>(+187.78%) | 273.56µs<br>(+1009.07%) | 532.74µs<br>(+2059.80%) | 247.59µs<br>(+903.76%) | 181.60µs<br>(+636.23%) |
| **small** | 13.01µs | 12.92µs<br>(±0%) | 11.67µs<br>(-10.29%) | 36.42µs<br>(+179.88%) | 82.31µs<br>(+532.49%) | 106.58µs<br>(+719.03%) | 49.99µs<br>(+284.19%) | 103.46µs<br>(+695.09%) |

#### Diff Minimality (number of edits produced)

| Test Case | znkr (baseline) | znkr-minimal | znkr-fast | go-internal | diffmatchpatch | godebug | mb0 | udiff |
|-----------|-----------------|--------------|-----------|-------------|----------------|---------|-----|-------|
| **large_01** | 5.615k edits | 5.615k edits<br>(±0%) | 5.615k edits<br>(±0%) | 5.617k edits<br>(+0.04%) | 5.615k edits<br>(±0%) | 5.615k edits<br>(±0%) | 5.615k edits<br>(±0%) | 19.04k edits<br>(+239.15%) |
| **large_02** | 28.87k edits | 28.83k edits<br>(-0.15%) | 31.80k edits<br>(+10.15%) | 31.81k edits<br>(+10.17%) | 28.83k edits<br>(-0.15%) | 28.83k edits<br>(-0.15%) | 28.83k edits<br>(-0.15%) | 31.75k edits<br>(+9.97%) |
| **large_03** | 5.504k edits | 5.504k edits<br>(±0%) | 5.504k edits<br>(±0%) | 5.506k edits<br>(+0.04%) | 5.504k edits<br>(±0%) | 5.504k edits<br>(±0%) | 5.504k edits<br>(±0%) | 55.69k edits<br>(+911.74%) |
| **large_04** | 26.99k edits | 26.99k edits<br>(-0.01%) | 27.80k edits<br>(+2.99%) | 27.80k edits<br>(+2.99%) | 60.36k edits<br>(+123.65%) | 26.99k edits<br>(-0.01%) | 26.99k edits<br>(-0.01%) | 100.94k edits<br>(+274.01%) |
| **medium** | 277 edits | 277 edits<br>(±0%) | 277 edits<br>(±0%) | 283 edits<br>(+2.17%) | 277 edits<br>(±0%) | 277 edits<br>(±0%) | 277 edits<br>(±0%) | 369 edits<br>(+33.21%) |
| **small** | 108 edits | 108 edits<br>(±0%) | 114 edits<br>(+5.56%) | 120 edits<br>(+11.11%) | 108 edits<br>(±0%) | 108 edits<br>(±0%) | 108 edits<br>(±0%) | 110 edits<br>(+1.85%) |

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
