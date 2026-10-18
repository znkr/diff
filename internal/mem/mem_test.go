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

package mem

import "testing"

func TestOf(t *testing.T) {
	b := []byte("abc")
	s := Of(b)
	c := s.Clone()
	b[0] = 'x'
	if s != "xbc" {
		t.Errorf("Of(b) = %q after changing b, want %q", s, "xbc")
	}
	if c != "abc" {
		t.Errorf("Of(b).Clone() = %q after changing b, want %q", c, "abc")
	}
	if got := Of("abc"); got != "abc" {
		t.Errorf("Of(%q) = %q", "abc", got)
	}
}

func TestOwn(t *testing.T) {
	b := []byte("abc")
	if got := Own[string](b); got != "abc" {
		t.Errorf("Own[string](b) = %q, want %q", got, "abc")
	}
	if got := Own[[]byte](b); &got[0] != &b[0] {
		t.Error("Own[[]byte](b) is a copy of b")
	}
}
