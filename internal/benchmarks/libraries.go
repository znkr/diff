package benchmarks

import (
	"bytes"
	"fmt"
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
	Name      string
	Diff      func(x, y []byte) []byte
	ParseOpts []textdiff.ParseOption // Options of ParseUnified that reads the output of Diff.
}

var Impls = []Impl{
	{
		Name: "znkr",
		Diff: func(x, y []byte) []byte {
			return textdiff.Unified(x, y, textdiff.IndentHeuristic(), textdiff.UnifiedHeader("x", "y"))
		},
	},
	{
		Name: "znkr-minimal",
		Diff: func(x, y []byte) []byte {
			return textdiff.Unified(x, y, diff.Minimal(), textdiff.IndentHeuristic(), textdiff.UnifiedHeader("x", "y"))
		},
	},
	{
		Name: "znkr-fast",
		Diff: func(x, y []byte) []byte {
			return textdiff.Unified(x, y, diff.Fast(), textdiff.IndentHeuristic(), textdiff.UnifiedHeader("x", "y"))
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

			return hunk(buf.Bytes())
		},
		ParseOpts: []textdiff.ParseOption{textdiff.ParseHeaderless()},
	},
	{
		Name: "godebug",
		Diff: func(x, y []byte) []byte {
			return hunk([]byte(godebug.Diff(string(x), string(y))))
		},
		ParseOpts: []textdiff.ParseOption{textdiff.ParseHeaderless()},
	},
	{
		Name: "mb0",
		Diff: func(x, y []byte) []byte {
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
			return hunk(buf.Bytes())
		},
		ParseOpts: []textdiff.ParseOption{textdiff.ParseHeaderless()},
	},
	{
		Name: "udiff",
		Diff: func(x, y []byte) []byte {
			return []byte(udiff.Unified("x", "y", string(x), string(y)))
		},
		// udiff writes hunks that overlap in y.
		ParseOpts: []textdiff.ParseOption{textdiff.ParseLenient()},
	},
}

// hunk returns body, a diff of every line of x and y, as one hunk of a unified
// diff.
func hunk(body []byte) []byte {
	if len(body) == 0 {
		return nil
	}
	// godebug and mb0 end body with a context line without a newline.
	if body[len(body)-1] != '\n' {
		body = append(body, '\n')
	}
	var n, m int
	for line := range bytes.Lines(body) {
		switch line[0] {
		case ' ':
			n++
			m++
		case '-':
			n++
		case '+':
			m++
		}
	}
	header := fmt.Appendf(nil, "@@ -%d,%d +%d,%d @@\n", min(n, 1), n, min(m, 1), m)
	return append(header, body...)
}

type mb0lines struct {
	x [][]byte
	y [][]byte
}

func (d mb0lines) Equal(i, j int) bool { return bytes.Equal(d.x[i], d.y[j]) }
