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

package byteview

import (
	"bytes"
	"slices"
	"testing"
	"unsafe"

	"github.com/google/go-cmp/cmp"
)

func TestFromString(t *testing.T) {
	str := "my string"

	got := From(str)
	if unsafe.StringData(got.data) != unsafe.StringData(str) {
		t.Errorf("From(str) points to different memory")
	}
	if got.Len() != len(str) {
		t.Errorf("got.Len() = %v, want %v", got.Len(), len(str))
	}

	t.Run("allocs", func(t *testing.T) {
		allocs := testing.AllocsPerRun(10, func() {
			_ = From(str)
		})
		if allocs > 0 {
			t.Errorf("From[string](...) allocated %v times, want 0", allocs)
		}
	})
}

func TestFromBytes(t *testing.T) {
	bytes := []byte("my byte slice")

	got := From(bytes)
	if unsafe.StringData(got.data) != unsafe.SliceData(bytes) {
		t.Errorf("From(bytes) points to different memory")
	}
	if got.Len() != len(bytes) {
		t.Errorf("got.Len() = %v, want %v", got.Len(), len(bytes))
	}

	t.Run("allocs", func(t *testing.T) {
		allocs := testing.AllocsPerRun(10, func() {
			_ = From(bytes)
		})
		if allocs > 0 {
			t.Errorf("From[[]byte](...) allocated %v times, want 0", allocs)
		}
	})
}

func TestByteViewBytes(t *testing.T) {
	b := []byte("my byte slice")

	got := slices.Collect(From(b).Bytes())
	if !bytes.Equal(got, b) {
		t.Errorf("From(b).Byte() = %q, want %q", got, b)
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		wantLines          []ByteView
		wantMissingNewline int
	}{
		{
			name:               "empty",
			input:              "",
			wantLines:          []ByteView{},
			wantMissingNewline: -1,
		},
		{
			name:               "newline-only",
			input:              "\n",
			wantLines:          []ByteView{From("\n")},
			wantMissingNewline: -1,
		},
		{
			name:               "missing-newline",
			input:              "foo\nbar",
			wantLines:          []ByteView{From("foo\n"), From("bar")},
			wantMissingNewline: 1,
		},
		{
			name:               "missing-newline-in-fist-line",
			input:              "foo",
			wantLines:          []ByteView{From("foo")},
			wantMissingNewline: 0,
		},
		{
			name:               "no-missing-newline",
			input:              "foo\nbar\nbaz\n",
			wantLines:          []ByteView{From("foo\n"), From("bar\n"), From("baz\n")},
			wantMissingNewline: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLines, gotMissingNewline := SplitLines(From(tt.input))
			if diff := cmp.Diff(tt.wantLines, gotLines, cmp.Transformer("byteview", func(v ByteView) string { return v.data })); diff != "" {
				t.Errorf("SplitLines(...) result difference [-got, +want]:\n%s", diff)
			}
			if gotMissingNewline != tt.wantMissingNewline {
				t.Errorf("SplitLines(...) returned missing newline at %v, want %v", gotMissingNewline, tt.wantMissingNewline)
			}
		})
	}
}

func TestBuilder(t *testing.T) {
	var b Builder[[]byte]
	b.WriteString("a")
	b.WriteByteView(From("b"))
	b.Write([]byte{'c'})

	got, want := b.Build(), []byte("abc")
	if !cmp.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}

	got, want = b.Build(), nil
	if !cmp.Equal(got, want) {
		t.Errorf("second call to Build: got %q, want %q", got, want)
	}
}

func TestBuilderBuildBytesAlloc(t *testing.T) {
	var b Builder[[]byte]
	allocs := testing.AllocsPerRun(10, func() {
		b.Grow(3)
		b.WriteString("a")
		b.WriteByteView(From("b"))
		b.Write([]byte{'c'})
		_ = b.Build()
	})
	if allocs > 1 {
		t.Errorf("Builder[...].Build() allocated %v times, want <= 1", allocs)
	}
}

func TestBuilderBuildStringAlloc(t *testing.T) {
	var b Builder[string]
	allocs := testing.AllocsPerRun(10, func() {
		b.Grow(3)
		b.WriteString("a")
		b.WriteByteView(From("b"))
		b.Write([]byte{'c'})
		_ = b.Build()
	})
	if allocs > 1 {
		t.Errorf("Builder[...].Build() allocated %v times, want <= 1", allocs)
	}
}
