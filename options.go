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

// Context sets the number of matches to include as a prefix and postfix for hunks returned in
// [Hunks] and [HunksFunc]. The default is 3.
func Context(n int) Option {
	return func(cfg *config.Config) {
		cfg.Context = max(0, n)
	}
}

// Optimal finds an optimal diff irrespective of the cost. By default, the comparison functions in
// this package limit the cost for large inputs with many differences by applying heuristics that
// reduce the time complexity.
//
// With this option, the runtime is O(ND) where N = len(x) + len(y), and D is the number of
// differences between x and y.
func Optimal() Option {
	return func(cfg *config.Config) {
		cfg.Optimal = true
	}
}
