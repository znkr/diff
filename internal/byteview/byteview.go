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

// Package byteview provides a mechanism to handle strings and []byte as immutable byte views.
package byteview

import (
	"iter"
	"slices"
	"strings"
	"sync"
	"unsafe"
)

type ByteView struct {
	data string
}

func From[T string | []byte](in T) ByteView {
	switch in := any(in).(type) {
	case string:
		return ByteView{in}
	case []byte:
		return ByteView{unsafe.String(unsafe.SliceData(in), len(in))}
	}
	panic("never reached")
}

func (v ByteView) Len() int { return len(v.data) }

func (v ByteView) Bytes() iter.Seq[byte] {
	return func(yield func(byte) bool) {
		for i := range len(v.data) {
			if !yield(v.data[i]) {
				break
			}
		}
	}
}

// SplitLines splits the input on '\n' and returns the lines including the newline character and
// and either -1 if the last line ends in a newline character or len([]ByteView) if it's missing
// a newline character.
func SplitLines(v ByteView) (lines []ByteView, missingNewline int) {
	s := v.data
	n := strings.Count(v.data, "\n")
	if len(s) > 0 && s[len(s)-1] != '\n' {
		n++
	}
	a := make([]ByteView, n)
	for i := range n {
		m := strings.Index(s, "\n")
		if m < 0 {
			break
		}
		a[i] = ByteView{s[:m+1]}
		s = s[m+1:]
	}
	missingNewline = -1
	if len(s) > 0 {
		a[n-1] = ByteView{s}
		missingNewline = n - 1
	}
	return a, missingNewline
}

type Builder[T string | []byte] struct {
	_   [0]sync.Mutex // don't copy
	buf []byte
}

func (b *Builder[T]) Grow(n int) {
	b.buf = slices.Grow(b.buf, n)
}

func (b *Builder[T]) Write(v []byte) (n int, err error) {
	b.buf = append(b.buf, v...)
	return len(v), nil
}

func (b *Builder[T]) WriteByteView(v ByteView) (n int, err error) {
	b.buf = append(b.buf, v.data...)
	return len(v.data), nil
}

func (b *Builder[T]) WriteString(v string) (n int, err error) {
	b.buf = append(b.buf, v...)
	return len(v), nil
}

func (b *Builder[T]) Build() T {
	defer func() {
		b.buf = nil
	}()
	switch any((*T)(nil)).(type) {
	case *string:
		return T(unsafe.String(unsafe.SliceData(b.buf), len(b.buf)))
	case *[]byte:
		return T(b.buf)
	}
	panic("never reached")
}
