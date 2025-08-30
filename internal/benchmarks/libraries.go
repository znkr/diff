package benchmarks

import (
	"bytes"
	"strings"

	"github.com/aymanbagabas/go-udiff"
	godebug "github.com/kylelemons/godebug/diff"
	mb0 "github.com/mb0/diff"
	gointernal "github.com/rogpeppe/go-internal/diff"
	"github.com/sergi/go-diff/diffmatchpatch"
	"znkr.io/diff"
	"znkr.io/diff/textdiff"
)

type Impl struct {
	Name string
	Diff func(x, y []byte) []byte
}

var Impls = []Impl{
	{
		Name: "znkr",
		Diff: func(x, y []byte) []byte {
			return textdiff.Unified(x, y, textdiff.IndentHeuristic())
		},
	},
	{
		Name: "znkr-optimal",
		Diff: func(x, y []byte) []byte {
			return textdiff.Unified(x, y, diff.Optimal(), textdiff.IndentHeuristic())
		},
	},
	{
		Name: "znkr-fast",
		Diff: func(x, y []byte) []byte {
			return textdiff.Unified(x, y, diff.Fast(), textdiff.IndentHeuristic())
		},
	},
	{
		Name: "go-internal",
		Diff: func(x, y []byte) []byte {
			return gointernal.Diff("x", x, "y", y)
		},
	},
	{
		Name: "diffmatchpatch",
		Diff: func(x, y []byte) []byte {
			// This function is not exactly creating a unified diff, but it's close enough to be
			// comparable.
			dmp := diffmatchpatch.New()
			rx, ry, lines := dmp.DiffLinesToRunes(string(x), string(y))
			diffs := dmp.DiffMainRunes(rx, ry, false)
			diffs = dmp.DiffCharsToLines(diffs, lines)

			var buf bytes.Buffer
			for _, diff := range diffs {
				text := diff.Text

				switch diff.Type {
				case diffmatchpatch.DiffInsert:
					lines := strings.SplitAfter(text, "\n")
					for _, line := range lines {
						if line == "" {
							continue
						}
						buf.WriteString("+")
						buf.WriteString(line)
					}

				case diffmatchpatch.DiffDelete:
					lines := strings.SplitAfter(text, "\n")
					for _, line := range lines {
						if line == "" {
							continue
						}
						buf.WriteString("-")
						buf.WriteString(line)
					}

				case diffmatchpatch.DiffEqual:
					lines := strings.SplitAfter(text, "\n")
					for _, line := range lines {
						if line == "" {
							continue
						}
						buf.WriteString(" ")
						buf.WriteString(line)
					}
				}
			}

			return buf.Bytes()
		},
	},
	{
		Name: "godebug",
		Diff: func(x, y []byte) []byte {
			// This function is not exactly creating a unified diff, but it's close enough to be
			// comparable.
			return []byte(godebug.Diff(string(x), string(y)))
		},
	},
	{
		Name: "mb0",
		Diff: func(x, y []byte) []byte {
			// This function is not exactly creating a unified diff, but it's close enough to be
			// comparable.
			d := mb0lines{
				x: bytes.SplitAfter(x, []byte("\n")),
				y: bytes.SplitAfter(y, []byte("\n")),
			}
			changes := mb0.Diff(len(d.x), len(d.y), d)
			var buf bytes.Buffer
			a, b := 0, 0
			for _, ch := range changes {
				for a < ch.A {
					buf.WriteString(" ")
					buf.Write(d.x[a])
					a++
					b++
				}
				for i := range ch.Del {
					buf.WriteString("-")
					buf.Write(d.x[ch.A+i])
					a++
				}
				for i := range ch.Ins {
					buf.WriteString("+")
					buf.Write(d.y[ch.B+i])
					b++
				}
			}
			for a < len(d.x) {
				buf.WriteString(" ")
				buf.Write(d.x[a])
				a++
			}
			return buf.Bytes()
		},
	},
	{
		Name: "udiff",
		Diff: func(x, y []byte) []byte {
			return []byte(udiff.Unified("x", "y", string(x), string(y)))
		},
	},
}

type mb0lines struct {
	x [][]byte
	y [][]byte
}

func (d mb0lines) Equal(i, j int) bool { return bytes.Equal(d.x[i], d.y[j]) }
