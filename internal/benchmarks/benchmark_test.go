package benchmarks

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	gointernal "github.com/rogpeppe/go-internal/diff"
	"github.com/sergi/go-diff/diffmatchpatch"
	"golang.org/x/tools/txtar"
	"znkr.io/diff"
	"znkr.io/diff/textdiff"
)

type impl struct {
	name string
	diff func(x, y []byte) []byte
	dist func(x, y []byte) int
}

var implementations = []impl{
	{
		name: "znkr",
		diff: func(x, y []byte) []byte {
			return textdiff.Unified(x, y)
		},
	},
	{
		name: "znkr-optimal",
		diff: func(x, y []byte) []byte {
			return textdiff.Unified(x, y, diff.Optimal())
		},
	},
	{
		name: "go-internal",
		diff: func(x, y []byte) []byte {
			return gointernal.Diff("x", x, "y", y)
		},
	},
	{
		name: "diffmatchpatch",
		diff: func(x, y []byte) []byte {
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
						buf.WriteString("+")
						buf.WriteString(line)
					}

				case diffmatchpatch.DiffDelete:
					lines := strings.SplitAfter(text, "\n")
					for _, line := range lines {
						buf.WriteString("-")
						buf.WriteString(line)
					}

				case diffmatchpatch.DiffEqual:
					// skip equal sections to emulate the unified format without any context
				}
			}

			return buf.Bytes()
		},
	},
}

type testdata struct {
	name string
	x, y []byte
}

func loadTestdata(t testing.TB) []testdata {
	t.Helper()
	testFiles, err := filepath.Glob("testdata/*.test")
	if err != nil {
		t.Fatalf("Failed to read testdata: %v", err)
	}
	var tests []testdata
	for _, filename := range testFiles {
		ar, err := txtar.ParseFile(filename)
		if err != nil {
			t.Fatalf("failed to parse test case: %v", err)
		}
		name := strings.TrimPrefix(filename, "testdata/")
		test := testdata{
			name: name,
		}

		for _, f := range ar.Files {
			switch f.Name {
			case "x":
				test.x = f.Data
			case "y":
				test.y = f.Data
			default:
				t.Fatalf("unknown file in archive: %v", f)
			}
		}
		tests = append(tests, test)
	}
	return tests
}

func BenchmarkDiffs(b *testing.B) {
	optD := make(map[string]int)
	for _, td := range loadTestdata(b) {
		edits := textdiff.Edits(td.x, td.y, diff.Optimal())
		d := 0
		for _, edit := range edits {
			if edit.Op != diff.Match {
				d++
			}
		}
		optD[td.name] = d
	}

	for _, impl := range implementations {
		b.Run("impl="+impl.name, func(b *testing.B) {
			for _, td := range loadTestdata(b) {
				b.Run("name="+td.name, func(b *testing.B) {
					for b.Loop() {
						_ = impl.diff(td.x, td.y)
					}
					b.StopTimer()

					out := impl.diff(td.x, td.y)
					edits := 0
					for _, line := range bytes.Split(out, []byte("\n")) {
						if bytes.HasPrefix(line, []byte{'+'}) || bytes.HasPrefix(line, []byte{'-'}) {
							edits++
						}
					}
					b.ReportMetric(float64(edits), "edits")
				})
			}
		})
	}
}
