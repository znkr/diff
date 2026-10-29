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
| **large_01** | 1.159ms | 7.658ms<br>(+560.66%) | 1.131ms<br>(-2.39%) | 4.035ms<br>(+248.14%) | 39.774ms<br>(+3331.36%) | 174.424ms<br>(+14947.80%) | 74.167ms<br>(+6298.50%) | 8.027ms<br>(+592.53%) |
| **large_02** | 17.242ms | 39.808ms<br>(+130.87%) | 0.968ms<br>(-94.38%) | 3.547ms<br>(-79.43%) | 566.932ms<br>(+3188.00%) | 2878.312ms<br>(+16593.18%) | 1289.645ms<br>(+7379.48%) | 8.676ms<br>(-49.68%) |
| **large_03** | 1.680ms | 10.837ms<br>(+545.19%) | 1.678ms<br>(±0%) | 3.823ms<br>(+127.63%) | 29.117ms<br>(+1633.55%) | 169.900ms<br>(+10015.25%) | 89.043ms<br>(+5201.35%) | 13.489ms<br>(+703.10%) |
| **large_04** | 4.352ms | 204.955ms<br>(+4609.26%) | 2.977ms<br>(-31.59%) | 6.825ms<br>(+56.83%) | 1012.962ms<br>(+23174.84%) | 16419.444ms<br>(+377169.93%) | 1894.043ms<br>(+43419.46%) | 22.867ms<br>(+425.41%) |
| **medium** | 23.03µs | 22.67µs<br>(-1.56%) | 22.64µs<br>(-1.69%) | 58.34µs<br>(+153.33%) | 244.37µs<br>(+961.15%) | 733.59µs<br>(+3085.50%) | 231.05µs<br>(+903.30%) | 185.45µs<br>(+705.27%) |
| **small** | 12.01µs | 11.91µs<br>(-0.82%) | 10.56µs<br>(-12.09%) | 31.62µs<br>(+163.35%) | 71.33µs<br>(+494.09%) | 190.14µs<br>(+1483.54%) | 49.79µs<br>(+314.68%) | 98.79µs<br>(+722.80%) |

#### Diff Minimality (number of edits produced)

| Test Case | znkr (baseline) | znkr-minimal | znkr-fast | go-internal | diffmatchpatch | godebug | mb0 | udiff |
|-----------|-----------------|--------------|-----------|-------------|----------------|---------|-----|-------|
| **large_01** | 5.615k edits | 5.615k edits<br>(±0%) | 5.615k edits<br>(±0%) | 5.615k edits<br>(±0%) | 5.615k edits<br>(±0%) | 5.615k edits<br>(±0%) | 5.615k edits<br>(±0%) | 19.04k edits<br>(+239.11%) |
| **large_02** | 28.87k edits | 28.83k edits<br>(-0.15%) | 31.80k edits<br>(+10.15%) | 31.80k edits<br>(+10.16%) | 28.83k edits<br>(-0.15%) | 28.83k edits<br>(-0.15%) | 28.83k edits<br>(-0.15%) | 31.75k edits<br>(+9.96%) |
| **large_03** | 5.504k edits | 5.504k edits<br>(±0%) | 5.504k edits<br>(±0%) | 5.504k edits<br>(±0%) | 5.504k edits<br>(±0%) | 5.504k edits<br>(±0%) | 5.504k edits<br>(±0%) | 55.68k edits<br>(+911.70%) |
| **large_04** | 26.99k edits | 26.99k edits<br>(-0.01%) | 27.80k edits<br>(+2.99%) | 27.80k edits<br>(+2.99%) | 60.07k edits<br>(+122.55%) | 26.99k edits<br>(-0.01%) | 26.99k edits<br>(-0.01%) | 100.94k edits<br>(+274.00%) |
| **medium** | 277 edits | 277 edits<br>(±0%) | 277 edits<br>(±0%) | 281 edits<br>(+1.44%) | 277 edits<br>(±0%) | 277 edits<br>(±0%) | 277 edits<br>(±0%) | 367 edits<br>(+32.49%) |
| **small** | 108 edits | 108 edits<br>(±0%) | 114 edits<br>(+5.56%) | 118 edits<br>(+9.26%) | 108 edits<br>(±0%) | 108 edits<br>(±0%) | 108 edits<br>(±0%) | 108 edits<br>(±0%) |

## Correctness

I tested this diff implementation against every commit in the [Go
repository](http://go.googlesource.com/go) using the standard unix `patch` tool to ensure that all
diff results are correct.

The eval command also checks the unified diff parser: `textdiff.ParseUnified` must return the hunks of each
diff that `textdiff.Unified` writes, and the hunks it reads from git's own diff of each commit must
turn the old file into the new one.

This test is part of the test suite for this module and can be run with

```
go run ./internal/cmd/eval -repo <repo>
```

## License

This module is distributed under the [Apache License, Version
2.0](https://www.apache.org/licenses/LICENSE-2.0), see [LICENSE](LICENSE) for more information.
