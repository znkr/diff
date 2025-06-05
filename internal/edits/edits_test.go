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
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"znkr.io/diff/internal/config"
)

func TestHunks(t *testing.T) {
	tests := []struct {
		name      string
		rx, ry    []bool
		context   int
		wantHunks []Hunk
		wantEdits int
	}{
		{
			name:      "empty",
			rx:        nil,
			ry:        nil,
			context:   3,
			wantHunks: nil,
			wantEdits: 0,
		},
		{
			name:    "ABCABBA_to_CBABAC_context_3",
			rx:      []bool{true, false, true, false, false, true, false, false},
			ry:      []bool{true, false, false, false, false, true, false},
			context: 3,
			wantHunks: []Hunk{
				{0, 7, 0, 6, 9},
			},
			wantEdits: 9,
		},
		{
			name:    "ABCABBA_to_CBABAC_context_1",
			rx:      []bool{true, false, true, false, false, true, false, false},
			ry:      []bool{true, false, false, false, false, true, false},
			context: 1,
			wantHunks: []Hunk{
				{0, 7, 0, 6, 9}, // overlapping hunks are merged
			},
			wantEdits: 9,
		},
		{
			name:    "ABCABBA_to_CBABAC_context_0",
			rx:      []bool{true, false, true, false, false, true, false, false},
			ry:      []bool{true, false, false, false, false, true, false},
			context: 0,
			wantHunks: []Hunk{
				{0, 1, 0, 1, 2},
				{2, 3, 2, 2, 1},
				{5, 6, 4, 4, 1},
				{7, 7, 5, 6, 1},
			},
			wantEdits: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slices.Collect(Hunks(tt.rx, tt.ry, config.Config{Context: tt.context}))
			if diff := cmp.Diff(tt.wantHunks, got); diff != "" {
				t.Errorf("Hunks(...) result are different [-want,+got]:\n%s", diff)
			}
		})
	}
}
