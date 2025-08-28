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

package diff

import "znkr.io/diff/internal/config"

// Option configures the behavior of comparison functions.
type Option = config.Option

// Context sets the number of unchanged elements to include around each hunk. The default is 3.
//
// Context anchors diffs in the surrounding context in addition to position information. For
// example, with Context(2), you'll see 2 unchanged elements before and after each group of changes.
//
// Only supported by functions that return hunks.
func Context(n int) Option {
	return func(cfg *config.Config) config.Flag {
		cfg.Context = max(0, n)
		return config.Context
	}
}

// Optimal ensures the diff algorithm finds the shortest possible diff by disabling performance
// heuristics.
//
// By default, the diff functions use heuristics to speed up computation for large inputs with many
// changes, which may produce slightly longer diffs. Use this option when you need the absolute
// shortest diff, at the cost of potentially slower performance.
//
// Performance impact: Changes time complexity from O(N^1.5 log N) to O(ND) where N = len(x) +
// len(y) and D is the number of differences.
func Optimal() Option {
	return func(cfg *config.Config) config.Flag {
		cfg.Mode = config.ModeOptimal
		return config.Optimal
	}
}

// Fast uses a heuristic to find a reasonable diff instead of trying to find a minimal diff.
//
// This option trades diff minimality for runtime performance. The resulting diff can be a lot
// larger than the diff created by default. The speedup from using [Fast] only really manifests for
// relatively few, very large inputs because the default already use the underlying heuristic to
// speed up large inputs.
//
// The heuristic only works for comparable types.
//
// Performance impact: This option changes the complexity to O(N log N).
func Fast() Option {
	return func(cfg *config.Config) config.Flag {
		cfg.Mode = config.ModeFast
		return config.Fast
	}
}
