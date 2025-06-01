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

package textdiff

import (
	"testing"
)

func TestUnifiedEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		x, y string
		want string
	}{
		{
			name: "empty",
			x:    "",
			y:    "",
			want: "",
		},
		{
			name: "identical",
			x:    "first line\n",
			y:    "first line\n",
			want: "",
		},
		{
			name: "new-lines-only",
			x:    "\n",
			y:    "\n",
			want: "",
		},
		{
			name: "x-empty",
			x:    "",
			y:    "one-line\n",
			want: "@@ -1,0 +1,1 @@\n+one-line\n",
		},
		{
			name: "y-empty",
			x:    "one-line\n",
			y:    "",
			want: "@@ -1,1 +1,0 @@\n-one-line\n",
		},
		{
			name: "missing-newline-x",
			x:    "first line",
			y:    "first line\n",
			want: "@@ -1,1 +1,1 @@\n-first line\n\\ No newline at end of file\n+first line\n",
		},
		{
			name: "missing-newline-y",
			x:    "first line\n",
			y:    "first line",
			want: "@@ -1,1 +1,1 @@\n-first line\n+first line\n\\ No newline at end of file\n",
		},
		{
			name: "missing-newline-both",
			x:    "a\nsecond line",
			y:    "b\nsecond line",
			want: "@@ -1,2 +1,2 @@\n-a\n+b\n second line\n\\ No newline at end of file\n",
		},
		{
			name: "missing-newline-empty-x",
			x:    "",
			y:    "\n",
			want: "@@ -1,0 +1,1 @@\n+\n", // no missing newline note here
		},
		{
			name: "missing-newline-empty-y",
			x:    "\n",
			y:    "",
			want: "@@ -1,1 +1,0 @@\n-\n", // no missing newline note here
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unified(tt.x, tt.y)
			if got != tt.want {
				t.Errorf("Unified(...) if different:\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}
