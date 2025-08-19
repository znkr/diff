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

//go:build experimental

package diff

import "znkr.io/diff/internal/config"

// AnchoringHeuristic enables a heuristic to anchor the diff around lines that are provably one 1:1
// correspondences in both files.
//
// The heuristic is similar to the patience diff algorithm, but it's not it's own algorithm. Instead
// it's used as a heuristic.
//
// Using this heuristic speeds up diffs for large files and produces better diffs than other
// heuristics we use to limit the time complexity of the algorithm.  However, this heuristic only
// works for comparable types.
//
// It's experimental, because it's unclear if this should be the default (maybe for large files) and
// because it's not as well tested.
func AnchoringHeuristic() Option {
	return func(cfg *config.Config) config.Flag {
		cfg.AnchoringHeuristic = true
		return config.AnchoringHeuristic
	}
}
