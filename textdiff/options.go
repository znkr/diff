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
	"znkr.io/diff/internal/config"
	"znkr.io/diff/textdiff/color"
)

// Option configures the behavior of comparison functions.
//
// Note: The [options defined in the diff package] work for most functions in
// this package too. See the description of the individual functions for a list
// of supported options.
//
// [options defined in the diff package]: https://pkg.go.dev/znkr.io/diff#Option
type Option = config.Option

// IndentHeuristic applies a heuristic to make diffs easier to read by improving
// the placement of edit boundaries.
//
// This implements a heuristic that shifts edit boundaries to align with
// indentation patterns, making the resulting diff more readable for humans. The
// heuristic is particularly effective with code and structured text.
func IndentHeuristic() Option {
	return func(cfg *config.Config) config.Flag {
		cfg.IndentHeuristic = true
		return config.IndentHeuristic
	}
}

// UnifiedHeader starts the output of a unified diff with a file header.
//
// The header has the format:
//
//	--- <x>
//	+++ <y>
//
// If both sides of the diff are equal, the output is empty and has no header.
//
// Labels are written so that git apply and GNU patch read them back
// unchanged:
//
//   - A label containing a control character, a double quote or a backslash
//     is written in double quotes with C-style escapes. BSD patch doesn't
//     decode such a label.
//   - Other labels are written as given, including non-ASCII bytes.
//   - A label containing a space is followed by a tab. Without it, patch reads
//     the file name only up to the first space.
//
// The output matches git's with core.quotePath set to false.
func UnifiedHeader(x, y string) Option {
	return func(c *config.Config) config.Flag {
		if c.Unified == nil {
			c.Unified = new(config.Unified)
		}
		c.Unified.Header = &config.UnifiedHeaderConfig{X: x, Y: y}
		return config.UnifiedHeader
	}
}

// TerminalColors uses ANSI escape codes to color the output of [Unified].
//
// By default, the colors try to emulate git's color scheme, but the colors can
// be overridden using [color.Option].
//
// Note: Using TerminalColors will output ANSI escape quotes unconditionally.
// It's the callers responsibility to make sure it's only used in contexts that
// support ANSI escape sequences (e.g. by guarding the use of this option with
// [github.com/mattn/go-isatty]).
//
// [github.com/mattn/go-isatty]: https://pkg.go.dev/github.com/mattn/go-isatty
func TerminalColors(opts ...color.Option) Option {
	return func(c *config.Config) config.Flag {
		if c.Unified == nil {
			c.Unified = new(config.Unified)
		}
		c.Unified.Color = config.ColorConfig{
			Reset:      "\033[m",
			FileHeader: "\033[1m",  // Bold
			HunkHeader: "\033[36m", // Cyan
			Match:      "",         // Normal
			Delete:     "\033[31m", // Red
			Insert:     "\033[32m", // Green
		}
		for _, opt := range opts {
			opt(&c.Unified.Color)
		}
		return config.TerminalColors
	}
}
