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
		name      string
		flags     []Flag
		n, m      int
		context   int
		wantHunks []Hunk
		wantEdits int
	}{
		{
			name:      "empty",
			flags:     nil,
			n:         0,
			m:         0,
			context:   3,
			wantHunks: nil,
			wantEdits: 0,
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
			wantHunks: []Hunk{
				{0, 7, 0, 6, 9},
			},
			wantEdits: 9,
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
			wantHunks: []Hunk{
				{0, 7, 0, 6, 9}, // overlapping hunks are merged
			},
			wantEdits: 9,
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
			gotHunks, gotEdits := Hunks(tt.flags, tt.n, tt.m, config.Config{Context: tt.context})
			if diff := cmp.Diff(tt.wantHunks, gotHunks); diff != "" {
				t.Errorf("Hunks(...) result are different [-want,+got]:\n%s", diff)
			}
			if gotEdits != tt.wantEdits {
				t.Errorf("Hunks(...) total edits is %v, want %v", gotEdits, tt.wantEdits)
			}
		})
	}
}
