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

// Package mem converts between strings and byte slices without copying.
package mem

import (
	"strings"
	"unsafe"
)

// UnsafeString is a string that may share memory with a []byte. It changes
// when the []byte changes, so it must not be kept after the []byte is
// modified. [UnsafeString.Clone] returns a string that can be kept.
//
// The methods of UnsafeString are those of package strings that do not keep
// their argument, so that it does not need to be converted to a string.
type UnsafeString string

// Of returns v as an UnsafeString without copying.
func Of[T string | []byte](v T) UnsafeString {
	switch v := any(v).(type) {
	case string:
		return UnsafeString(v)
	case []byte:
		return UnsafeString(unsafe.String(unsafe.SliceData(v), len(v)))
	}
	panic("never reached")
}

// Clone returns a copy of s.
func (s UnsafeString) Clone() string { return strings.Clone(string(s)) }

// IndexByte returns the index of the first c in s, or -1 if there is none.
func (s UnsafeString) IndexByte(c byte) int { return strings.IndexByte(string(s), c) }

// LastIndexByte returns the index of the last c in s, or -1 if there is none.
func (s UnsafeString) LastIndexByte(c byte) int { return strings.LastIndexByte(string(s), c) }

// Count returns the number of non-overlapping instances of substr in s.
func (s UnsafeString) Count(substr string) int { return strings.Count(string(s), substr) }

// Own returns b as a T without copying. b must not be modified afterwards.
func Own[T string | []byte](b []byte) T {
	switch any((*T)(nil)).(type) {
	case *string:
		return T(unsafe.String(unsafe.SliceData(b), len(b)))
	case *[]byte:
		return T(b)
	}
	panic("never reached")
}
