package benchmarks

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"
	"znkr.io/diff"
	"znkr.io/diff/textdiff"
)

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
		edits := textdiff.Edits(td.x, td.y, diff.Minimal())
		d := 0
		for _, edit := range edits {
			if edit.Op != diff.Match {
				d++
			}
		}
		optD[td.name] = d
	}

	for _, impl := range Impls {
		b.Run("impl="+impl.Name, func(b *testing.B) {
			for _, td := range loadTestdata(b) {
				b.Run("name="+td.name, func(b *testing.B) {
					for b.Loop() {
						_ = impl.Diff(td.x, td.y)
					}
					b.StopTimer()

					out := impl.Diff(td.x, td.y)
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
