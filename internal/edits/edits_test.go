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

package edits

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"znkr.io/diff/internal/config"
)

func TestHunks(t *testing.T) {
	tests := []struct {
		name    string
		flags   []Flag
		n, m    int
		context int
		want    []Hunk
	}{
		{
			name:    "empty",
			flags:   nil,
			n:       0,
			m:       0,
			context: 3,
			want:    nil,
		},
		{
			name: "ABCABBA_to_CBABAC_context_3",
			flags: []Flag{
				Insert | Delete, // -A +C
				None,            //  B  B
				Delete,          // -C  A
				None,            //  A  B
				None,            //  B  A
				Insert | Delete, // -B +C
				None,            //  A
				None,            // border
			},
			n:       7,
			m:       6,
			context: 3,
			want: []Hunk{
				{0, 7, 0, 6},
			},
		},
		{
			name: "ABCABBA_to_CBABAC_context_1",
			flags: []Flag{
				Insert | Delete, // -A +C
				None,            //  B  B
				Delete,          // -C  A
				None,            //  A  B
				None,            //  B  A
				Insert | Delete, // -B +C
				None,            //  A
				None,            // border
			},
			n:       7,
			m:       6,
			context: 1,
			want: []Hunk{
				{0, 7, 0, 6}, // overlapping hunks are merged
			},
		},
		{
			name: "ABCABBA_to_CBABAC_context_0",
			flags: []Flag{
				Insert | Delete, // -A +C
				None,            //  B  B
				Delete,          // -C  A
				None,            //  A  B
				None,            //  B  A
				Insert | Delete, // -B +C
				None,            //  A
				None,            // border
			},
			n:       7,
			m:       6,
			context: 0,
			want: []Hunk{
				{0, 1, 0, 1},
				{2, 3, 2, 2},
				{5, 6, 4, 4},
				{7, 7, 5, 6},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Hunks(tt.flags, tt.n, tt.m, config.Config{Context: tt.context})
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Hunks(...) result are different [-want,+got]:\n%s", diff)

			}
		})
	}
}
