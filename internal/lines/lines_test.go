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

package lines

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"znkr.io/diff/internal/benchdata"
)

func TestSplit(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines []string
	}{
		{
			name:      "empty",
			input:     "",
			wantLines: []string{},
		},
		{
			name:      "newline-only",
			input:     "\n",
			wantLines: []string{"\n"},
		},
		{
			name:      "missing-newline",
			input:     "foo\nbar",
			wantLines: []string{"foo\n", "bar"},
		},
		{
			name:      "missing-newline-in-first-line",
			input:     "foo",
			wantLines: []string{"foo"},
		},
		{
			name:      "no-missing-newline",
			input:     "foo\nbar\nbaz\n",
			wantLines: []string{"foo\n", "bar\n", "baz\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := Split(tt.input)
			defer l.Release()
			got := all(&l)
			if diff := cmp.Diff(tt.wantLines, got); diff != "" {
				t.Errorf("Split(%q) lines differ [-want, +got]:\n%s", tt.input, diff)
			}

			lb := Split([]byte(tt.input))
			defer lb.Release()
			if diff := cmp.Diff(tt.wantLines, all(&lb)); diff != "" {
				t.Errorf("Split([]byte(%q)) lines differ [-want, +got]:\n%s", tt.input, diff)
			}
		})
	}
}

func TestSplitPair(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	gen := func(n int) string {
		var b strings.Builder
		for range n {
			b.WriteString([]string{"a", "b", "\n", "ab\n", "\n\n", "a\nb"}[rng.IntN(6)])
		}
		return b.String()
	}
	for range 5000 {
		pre, suf := gen(rng.IntN(6)), gen(rng.IntN(6))
		x := pre + gen(rng.IntN(4)) + suf
		y := pre + gen(rng.IntN(4)) + suf
		xl, yl, prefix, suffix := SplitPair(x, y)
		wx, wy := Split(x), Split(y)
		checkLines(t, &xl, &wx, rng)
		checkLines(t, &yl, &wy, rng)
		gotX, gotY := all(&wx), all(&wy)

		if prefix+suffix > min(len(gotX), len(gotY)) {
			t.Fatalf("SplitPair(%q, %q): prefix %d and suffix %d overlap", x, y, prefix, suffix)
		}
		for i := range prefix {
			if gotX[i] != gotY[i] {
				t.Fatalf("SplitPair(%q, %q): prefix line %d differs", x, y, i)
			}
		}
		for i := 1; i <= suffix; i++ {
			if gotX[len(gotX)-i] != gotY[len(gotY)-i] {
				t.Fatalf("SplitPair(%q, %q): suffix line -%d differs", x, y, i)
			}
		}
		if x == y && prefix+suffix != len(gotX) {
			t.Fatalf("SplitPair(%q, %q) = %d, %d, want all %d lines", x, y, prefix, suffix, len(gotX))
		}
		// The prefix and suffix are maximal: the first and last lines between
		// them differ.
		if prefix+suffix < min(len(gotX), len(gotY)) {
			if gotX[prefix] == gotY[prefix] {
				t.Fatalf("SplitPair(%q, %q): prefix %d isn't maximal", x, y, prefix)
			}
			if gotX[len(gotX)-1-suffix] == gotY[len(gotY)-1-suffix] {
				t.Fatalf("SplitPair(%q, %q): suffix %d isn't maximal", x, y, suffix)
			}
		}
		xb, yb, pb, sb := SplitPair([]byte(x), []byte(y))
		if pb != prefix || sb != suffix {
			t.Fatalf("SplitPair([]byte(%q), []byte(%q)) = %d, %d, want %d, %d", x, y, pb, sb, prefix, suffix)
		}
		for _, l := range []*Lines{&xl, &yl, &wx, &wy} {
			l.Release()
		}
		xb.Release()
		yb.Release()
	}
}

// checkLines compares the lines and sizes of l with those of want, accessing
// them in random order so that l computes its boundaries in different orders.
func checkLines(t *testing.T, l, want *Lines, rng *rand.Rand) {
	t.Helper()
	if l.Len() != want.Len() {
		t.Fatalf("Len() = %d, want %d", l.Len(), want.Len())
	}
	for range 3 {
		i := rng.IntN(l.Len() + 1)
		j := i + rng.IntN(l.Len()-i+1)
		if got, w := l.Size(i, j), want.Size(i, j); got != w {
			t.Fatalf("Size(%d, %d) = %d, want %d", i, j, got, w)
		}
	}
	for _, i := range rng.Perm(l.Len()) {
		if got, w := l.Line(i), want.Line(i); got != w {
			t.Fatalf("Line(%d) = %q, want %q", i, got, w)
		}
	}
}

func TestSplitPairExamples(t *testing.T) {
	tests := []struct {
		x, y                   string
		wantPrefix, wantSuffix int
	}{
		{"", "", 0, 0},
		{"a\n", "a\n", 1, 0},
		{"a", "a", 0, 1},
		{"a", "ab", 0, 0},
		{"b", "ab", 0, 0},
		{"a\nb\nc\n", "a\nx\nc\n", 1, 1},
		{"a\nbc\n", "a\nc\n", 1, 0},
		{"a\nb", "a\nc\nb", 1, 1},
		{"a\na\n", "a\n", 1, 0},
	}
	for _, tt := range tests {
		xl, yl, prefix, suffix := SplitPair(tt.x, tt.y)
		if prefix != tt.wantPrefix || suffix != tt.wantSuffix {
			t.Errorf("SplitPair(%q, %q) = %d, %d, want %d, %d", tt.x, tt.y, prefix, suffix, tt.wantPrefix, tt.wantSuffix)
		}
		xl.Release()
		yl.Release()
	}
}

// all returns all lines of l.
func all(l *Lines) []string {
	out := make([]string, l.Len())
	for i := range out {
		out[i] = l.Line(i)
	}
	return out
}

// BenchmarkSplitPair runs SplitPair on inputs of package benchdata.
func BenchmarkSplitPair(b *testing.B) {
	for _, name := range []string{"small", "medium", "large01", "large04", "one"} {
		b.Run(name, func(b *testing.B) {
			x, y := benchdata.Load(b, name)
			b.ReportAllocs()
			for b.Loop() {
				xl, yl, _, _ := SplitPair(x, y)
				xl.Release()
				yl.Release()
			}
		})
	}
}
