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
// Note: The [options defined in the diff package] work for most functions in this package too. See
// the description of the individual functions for a list of supported options.
//
// [options defined in the diff package]: https://pkg.go.dev/znkr.io/diff#Option
type Option = config.Option

// IndentHeuristic applies a heuristic to make diffs easier to read by improving the placement of
// edit boundaries.
//
// This implements a heuristic that shifts edit boundaries to align with indentation patterns,
// making the resulting diff more readable for humans. The heuristic is particularly effective with
// code and structured text.
func IndentHeuristic() Option {
	return func(cfg *config.Config) config.Flag {
		cfg.IndentHeuristic = true
		return config.IndentHeuristic
	}
}

// TerminalColors uses ANSI escape codes to color the output of [Unified].
//
// By default, the colors try to emulate git's color scheme, but the colors can be overridden using
// [color.Option].
//
// Note: Using TerminalColors will output ANSI escape quotes unconditionally. It's the callers
// responsibility to make sure it's only used in contexts that support ANSI escape sequences (e.g.
// by guarding the use of this option with [github.com/mattn/go-isatty]).
//
// [github.com/mattn/go-isatty]: https://pkg.go.dev/github.com/mattn/go-isatty
func TerminalColors(opts ...color.Option) Option {
	return func(c *config.Config) config.Flag {
		colors := config.ColorConfig{
			Reset:      "\033[m",
			HunkHeader: "\033[36m", // Cyan
			Match:      "",         // Normal
			Delete:     "\033[31m", // Red
			Insert:     "\033[32m", // Green
		}
		for _, opt := range opts {
			opt(&colors)
		}
		c.Colors = &colors
		return config.TerminalColors
	}
}
