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

package config_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"znkr.io/diff"
	"znkr.io/diff/internal/config"
	"znkr.io/diff/textdiff"
)

func TestFromOptions(t *testing.T) {
	tests := []struct {
		name string
		opts []config.Option
		want config.Config
	}{
		{
			name: "default",
			opts: nil,
			want: config.Default,
		},
		{
			name: "context",
			opts: []config.Option{
				diff.Context(5),
			},
			want: config.Config{
				Context:         5,
				Optimal:         config.Default.Optimal,
				IndentHeuristic: config.Default.IndentHeuristic,
			},
		},
		{
			name: "optimal",
			opts: []config.Option{
				diff.Optimal(),
			},
			want: config.Config{
				Context:         config.Default.Context,
				Optimal:         true,
				IndentHeuristic: config.Default.IndentHeuristic,
			},
		},
		{
			name: "optimal-context",
			opts: []config.Option{
				diff.Optimal(),
				diff.Context(5),
			},
			want: config.Config{
				Context:         5,
				Optimal:         true,
				IndentHeuristic: config.Default.IndentHeuristic,
			},
		},
		{
			name: "context-override",
			opts: []config.Option{
				diff.Context(5),
				diff.Optimal(),
				diff.Context(1),
			},
			want: config.Config{
				Context:         1,
				Optimal:         true,
				IndentHeuristic: config.Default.IndentHeuristic,
			},
		},
		{
			name: "everything",
			opts: []config.Option{
				diff.Context(5),
				diff.Optimal(),
				textdiff.IndentHeuristic(),
			},
			want: config.Config{
				Context:         5,
				Optimal:         true,
				IndentHeuristic: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := config.FromOptions(tt.opts, config.Context|config.Optimal|config.IndentHeuristic)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("FromOptions(...) result are different [-want,+got]:\n%s", diff)
			}
		})
	}
}
