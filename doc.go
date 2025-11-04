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

// Package diff provides functions to efficiently compare two slices similar to the Unix diff
// command line tool to compare files.
//
// The main functions are [Hunks], which groups changes into contextual blocks, and [Edits], which
// returns every individual change. By default, the algorithms are optimized for performance and may
// use heuristics for very large inputs. Use [Minimal] to disable these heuristics when you need the
// shortest possible diff.
//
// Performance: Default complexity is O(N^1.5 log N) time and O(N) space. With [Minimal], time
// complexity is O(ND) where N = len(x) + len(y) and D is the number of edits. With [Fast], time
// complexity is O(N log N).
//
// Note: For a line-by-line diff of text, please see [znkr.io/diff/textdiff].
//
// [znkr.io/diff/textdiff]: https://pkg.go.dev/znkr.io/diff/textdiff
package diff
