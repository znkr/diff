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
	"znkr.io/diff"
	"znkr.io/diff/internal/config"
)

// IndentHeuristic applies a heuristic to make diffs easier to read by improving the placement of
// edit boundaries.
//
// This implements a heuristic that shifts edit boundaries to align with indentation patterns,
// making the resulting diff more readable for humans. The heuristic is particularly effective with
// code and structured text.
func IndentHeuristic() diff.Option {
	return func(cfg *config.Config) config.Flag {
		cfg.IndentHeuristic = true
		return config.IndentHeuristic
	}
}
