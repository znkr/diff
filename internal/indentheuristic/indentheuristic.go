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

// Package indentheuristic is an implementation of the indentation heuristic by Michael Haggerty
// (https://github.com/mhagger/diff-slider-tools).
//
// The idea behind the heuristic is that, since there's usually not a single solution, we can
// locally vary the solution we found to improve the aesthetics. We can vary the solution around the
// following degrees of freedom:
//
//	a. A deletion of a line X, followed by an insertion of line Y is the same as an insertion of
//	   line Y followed by a deletion of line X.
//	b. A deletion of a line X, followed by zero or more deletions, followed by a match Y with
//	   X == Y allows us to swap the deletion and the match. The same is true for insertions. The
//	   same is true the other way around (match of line X, followed by zero or more deletions,
//	   followed by a deletion of line Y with X == Y).
//
// The heuristics use these degrees of freedom to achieve the following goals:
//
//  1. Group deletions and insertions using (we do this implicitly by how we construct the diff
//     from the result slices).
//  2. Make deletion and insertion groups as large as possible by merging adjacent groups if
//     using.
//  3. If possible align deletions and insertions such that deletions are followed by insertions
//     without a matching line in between using.
//  4. If it's not possible to align deletions and insertions, shift beginnings and ends of
//     deletions / insertion groups to a line that has significance to humans based on indention at
//     the line and around the line.
//
// The most intricate piece of these heuristics are in (4) which is based on human rated diffs.
package indentheuristic

import (
	"cmp"

	"znkr.io/diff/internal/byteview"
)

// Never move a group more than this many lines.
const maxSliding = 100

// We don't care if a line is indented more than this and clamp the value to maxIndent. That way,
// we don't overflow an int and avoid unnecessary work on input that's not human readable text.
const maxIndent = 200

// Don't consider more than this number of consecutive blank lines. This is to bound the work
// and avoid integer overflows.
const maxBlanks = 20

const startOfFilePenalty = 1               // No no-blank lines before the split
const endOfFilePenalty = 21                // No non-blank lines after the split
const totalBlankWeight = -30               // Weight for number of blank lines around the split
const postBlankWeight = 6                  // Weight for number of blank lines after the split
const relativeIndentPenalty = -4           // Indented more than predecessor
const relativeIndentWithBlankPenalty = 10  // Indented more than predecessor, with blank lines
const relativeOutdentPenalty = 24          // Indented less than predecessor
const relativeOutdentWithBlankPenalty = 17 // Indented less than predecessor, with blank lines
const relativeDentPenalty = 23             // Indented less than predecessor but not less than successor
const relativeDentWithBlankPenalty = 17    // Indented less than predecessor but not less than successor, with blank lines

// We only consider whether the sum of the effective indents for splits are less than (-1), equal
// to (0), or greater than (+1) each other. The resulting value is multiplied by the following
// weight and combined with the penalty to determine the better of two scores.
const indentWeight = 60

// Apply applies the indent heuristics to rx and ry.
func Apply(x, y []byteview.ByteView, rx, ry []bool) {
	apply0(x, y, rx, ry) // for deletions
	apply0(y, x, ry, rx) // for insertions
}

// apply0 applies the indentation heuristics to r.
func apply0(lines, lineso []byteview.ByteView, r, ro []bool) {
	s, so := newScanner(lines, r), newScanner(lineso, ro)
	for s.nextGroup() {
		if !so.nextGroup() {
			panic("scanner sync broken")
		}

		if s.groupLen() == 0 {
			continue
		}

		matchingEnd := -1 // End of group that aligns with other input.
		minEnd := s.end   // Highest line that the group can be shifted to.
		grpLen := 0
		for grpLen != s.groupLen() {
			grpLen = s.groupLen()
			matchingEnd = -1

			// Slide up as much as possible and merge with adjacent groups.
			for s.slideGroupUp() {
				if !so.prevGroup() {
					panic("scanner sync broken")
				}
			}

			minEnd = s.end
			if so.groupLen() > 0 {
				matchingEnd = s.end
			}

			// Slide down as much as possible and merge with adjacent groups.
			for s.slideGroupDown() {
				if !so.nextGroup() {
					panic("scanner sync broken")
				}
				if so.groupLen() > 0 {
					matchingEnd = s.end
				}
			}
		}

		switch {
		case minEnd == s.end:
			// no shifting possible
		case matchingEnd != -1:
			// found a matching group, align with it
			for so.groupLen() == 0 {
				if !s.slideGroupUp() {
					panic("match disappeared")
				}
				if !so.prevGroup() {
					panic("scanner sync broken")
				}
			}
		default:
			// The group can be shifted around somewhat, we can use the possible shift range to
			// apply heuristics that make the diff easier to read. Right now, the group is shifted
			// to it's lowest position, so we only have to consider upward shifts.

			bestShift := -1
			var bestScore shiftScore
			for shift := max(minEnd, s.end-grpLen-1, s.end-maxSliding); shift <= s.end; shift++ {
				score := shiftScore{}
				score.add(measureShift(lines, shift))
				score.add(measureShift(lines, shift-grpLen))
				if bestShift == -1 || score.cmp(bestScore) <= 0 {
					bestShift = shift
					bestScore = score
				}
			}

			for s.end > bestShift {
				if !s.slideGroupUp() {
					panic("best shift not found")
				}
				if !so.prevGroup() {
					panic("scanner sync broken")
				}
			}
		}
	}

	if so.nextGroup() {
		panic("scanner sync broken")
	}
}

type scanner struct {
	start int // First changed line of the current group if non-empty, or unchanged line if empty.
	end   int // First unchanged line after the group. For an empty group, start == end.
	lines []byteview.ByteView
	r     []bool
}

func newScanner(lines []byteview.ByteView, r []bool) *scanner {
	return &scanner{
		start: -1,
		end:   -1,
		lines: lines,
		r:     r,
	}
}

// groupLen returns the length of the current group.
func (s *scanner) groupLen() int { return s.end - s.start }

// nextGroup moves s to the nextGroup (possibly empty) group and returns true. Returns false if
// the end is reached.
func (s *scanner) nextGroup() bool {
	if s.end == len(s.r)-1 {
		return false
	}
	s.start, s.end = s.end+1, s.end+1
	for s.end < len(s.r)-1 && s.r[s.end] {
		s.end++
	}
	return true
}

// prevGroup moves g to the previous (possibly empty) group and return true. Returns true if the
// beginning is reached.
func (s *scanner) prevGroup() bool {
	if s.start == 0 {
		return false
	}
	s.start, s.end = s.start-1, s.start-1
	for s.start > 0 && s.r[s.start-1] {
		s.start--
	}
	return true
}

// slideGroupDown tried to slide g down by one. If the slide up connects g with another group at below
// it, it merges the two groups. Returns true if sliding up was possible and false if the group
// could not be slid up.
func (s *scanner) slideGroupDown() bool {
	if s.end < len(s.r)-1 && s.lines[s.start] == s.lines[s.end] {
		s.r[s.start], s.r[s.end] = false, true
		s.start++
		s.end++
		for s.end < len(s.r)-1 && s.r[s.end] {
			s.end++
		}
		return true
	} else {
		return false
	}
}

// slideGroupUp tries to slide g up by one. If the slide up connects g with another group above it, it
// merges the two groups. Returns true if sliding up was possible and false if the group could not
// be slid up.
func (s *scanner) slideGroupUp() bool {
	if s.start > 0 && s.lines[s.start-1] == s.lines[s.end-1] {
		s.r[s.start-1], s.r[s.end-1] = true, false
		s.start--
		s.end--
		for s.start > 0 && s.r[s.start-1] {
			s.start--
		}
		return true
	} else {
		return false
	}
}

type measure struct {
	endOfFile  bool
	indent     int
	preBlank   int
	preIndent  int
	postBlank  int
	postIndent int
}

func measureShift(lines []byteview.ByteView, shift int) measure {
	m := measure{}
	if shift >= len(lines) {
		m.endOfFile = true
		m.indent = -1
	} else {
		m.indent = getIndent(lines[shift])
	}

	m.preIndent = -1
	for i := shift - 1; i >= 0; i-- {
		m.preIndent = getIndent(lines[i])
		if m.preIndent != -1 {
			break
		}
		m.preBlank++
		if m.preBlank == maxBlanks {
			m.preIndent = 0
			break
		}
	}

	m.postIndent = -1
	for i := shift + 1; i < len(lines); i++ {
		m.postIndent = getIndent(lines[i])
		if m.postIndent != -1 {
			break
		}
		m.postBlank++
		if m.postBlank == maxBlanks {
			m.postIndent = 0
			break
		}
	}
	return m
}

func getIndent(line byteview.ByteView) int {
	indent := 0
	for c := range line.Bytes() {
		switch c {
		case ' ':
			indent++
		case '\t':
			indent += 8 - indent%8
		case '\n', '\v', '\r':
			// Ignore other whitespace.
		default:
			return indent
		}
		if indent >= maxIndent {
			return maxIndent
		}
	}
	return -1 // only whitespace
}

type shiftScore struct {
	effectiveIndent int // smaller is better
	penalty         int // smaller is better
}

func (s *shiftScore) add(m measure) {
	if m.preIndent == 1 && m.preBlank == 0 {
		s.penalty += startOfFilePenalty
	}
	if m.endOfFile {
		s.penalty += endOfFilePenalty
	}

	postBlank := 0
	if m.indent == -1 {
		postBlank = 1 + m.postBlank
	}
	totalBlank := m.preBlank + postBlank

	// Penalties based on nearby blank lines
	s.penalty += totalBlankWeight * totalBlank
	s.penalty += postBlankWeight * postBlank

	indent := m.indent
	if indent == -1 {
		indent = m.postIndent
	}

	s.effectiveIndent += indent

	if indent == -1 || m.preIndent == -1 {
		// No additional adjustment needed.
	} else if indent > m.preIndent {
		// The line is indented more than it's predecessors.
		if totalBlank != 0 {
			s.penalty += relativeIndentWithBlankPenalty
		} else {
			s.penalty = relativeIndentPenalty
		}
	} else if indent == m.preIndent {
		// Same indentation as previous line, no adjustments need.
	} else {
		// Line is indented more than it's predecessor. It could be the block terminator of the
		// previous block, but it could also be the start of a new block (e.g., an "else" block, or
		// maybe the previous block didn't have a block terminator). Try to distinguish those cases
		// based on what comes next.
		if m.postIndent != -1 && m.postIndent > indent {
			// The following line is indented more. So it's likely that this line is the start of a
			// block.
			if totalBlank != 0 {
				s.penalty += relativeOutdentWithBlankPenalty
			} else {
				s.penalty += relativeOutdentPenalty
			}
		} else {
			if totalBlank != 0 {
				s.penalty += relativeDentWithBlankPenalty
			} else {
				s.penalty += relativeDentPenalty
			}
		}
	}
}

func (s *shiftScore) cmp(t shiftScore) int {
	return indentWeight*cmp.Compare(s.effectiveIndent, t.effectiveIndent) + s.penalty - t.penalty
}
