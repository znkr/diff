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
// command line tool to compares files.
//
// By default the comparison functions in this package will try to find an optimal path, but may
// fall back to a good-enough path for large files with many differences to speed up the comparison.
// Unless [Optimal] is used to disable these heuristics, the time complexity is O(N^1.5 log N) and
// the space complexity is O(N) with N = len(x) + len(y). With [Optimal] the complexity becomes
// O(ND) where D is the number of edits.
package diff
