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

package impl

// minCostLimit is a lower bound for the TOO_EXPENSIVE heuristic. That is the
// heuristic is only applied when the cost exceeds this number (large files with
// a lot of differences).
const minCostLimit = 4096

// Constants for GOOD_DIAGONAL heuristic.
const (
	// Minimal length of a diagonal for it to be considered.
	goodDiagMinLen = 20

	// The Heuristic is only applied if the cost exceeds this number.
	goodDiagCostLimit = 256

	// Magic number for diagonal selection.
	goodDiagMagic = 4
)

// Constants for ANCHORING heuristic.
const (
	// Minimum length for enabling the anchoring heuristic.
	anchoringHeuristicMinInputLen = 5_000
)
