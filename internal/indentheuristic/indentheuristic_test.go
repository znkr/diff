// Copyright 2025 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package indentheuristic

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/tools/txtar"
)

func TestApply(t *testing.T) {
	tests, err := filepath.Glob("testdata/*.test")
	if err != nil {
		t.Fatalf("Failed to read testdata: %v", err)
	}
	for _, test := range tests {
		name := strings.TrimPrefix(test, "testdata/")
		t.Run(name, func(t *testing.T) {
			ar, err := txtar.ParseFile(test)
			if err != nil {
				t.Fatalf("failed to parse test case: %v", err)
			}

			var input, want []byte
			for _, f := range ar.Files {
				switch f.Name {
				case "input":
					input = f.Data
				case "want":
					want = f.Data
				default:
					t.Fatalf("unknown file in archive: %v", f)
				}
			}

			x, y, rx, ry := parse(t, input)
			Apply(x, y, rx, ry)
			got := render(x, y, rx, ry)

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("indent heuristic produced different result.\ngot:\n%s\nwant:\n%s\ndiff\n%s", got, want, diff)
			}
		})
	}
}

func parse(t *testing.T, diff []byte) (x, y [][]byte, rx, ry []bool) {
	for line := range bytes.Lines(diff) {
		switch line[0] {
		case ' ':
			x = append(x, line[1:])
			y = append(y, line[1:])
			rx = append(rx, false)
			ry = append(ry, false)
		case '-':
			x = append(x, line[1:])
			rx = append(rx, true)
		case '+':
			y = append(y, line[1:])
			ry = append(ry, true)
		default:
			t.Fatalf("failed to parse diff: unknown prefix %q", line[0])
		}
	}
	// Border
	rx = append(rx, false)
	ry = append(ry, false)
	return
}

func render(x, y [][]byte, rx, ry []bool) []byte {
	var b bytes.Buffer
	for s, t := 0, 0; s < len(x) || t < len(y); {
		for s < len(x) && rx[s] {
			b.WriteString("-")
			b.Write(x[s])
			s++
		}
		for t < len(y) && ry[t] {
			b.WriteString("+")
			b.Write(y[t])
			t++
		}
		for s < len(x) && t < len(y) && !rx[s] && !ry[t] {
			b.WriteString(" ")
			b.Write(x[s])
			s++
			t++
		}
	}
	return b.Bytes()
}
