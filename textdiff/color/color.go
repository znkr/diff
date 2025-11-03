// Package color provides configuration for coloring for unified diffs using ANSI escape sequences.
//
// Specifying colors uses [Select Graphic Rendition parameters]. For example the code below,
// presents the header in bold yellow:
//
//	HunkHeader(1, 33)
//
// This is equivalent to the following raw ANSI sequence: \033[1;33m.
//
// It's the responsibility of the caller to ensure that the parameters are correct and supported
// by the underlying terminal.
//
// [Select Graphic Rendition parameters]: https://en.wikipedia.org/wiki/ANSI_escape_code#SGR
package color

import (
	"fmt"
	"strings"

	"znkr.io/diff/internal/config"
)

// A Option makes it possible to configure custom colors in [TerminalColors].
type Option func(*config.ColorConfig)

// HunkHeaders colors hunk headers, the "@@ ... @@" part of the unified diff.
func HunkHeaders(params ...int) Option {
	code := format(params)
	return func(cc *config.ColorConfig) {
		cc.HunkHeader = code
	}
}

// Matches colors matching lines.
func Matches(params ...int) Option {
	code := format(params)
	return func(cc *config.ColorConfig) {
		cc.Match = code
	}
}

// Deletes colors deleted lines.
func Deletes(params ...int) Option {
	code := format(params)
	return func(cc *config.ColorConfig) {
		cc.Delete = code
	}
}

// Inserts colors deleted lines.
func Inserts(params ...int) Option {
	code := format(params)
	return func(cc *config.ColorConfig) {
		cc.Insert = code
	}
}

func format(params []int) string {
	var sb strings.Builder
	sb.WriteString("\033[")
	for i, v := range params {
		if i > 0 {
			sb.WriteRune(';')
		}
		fmt.Fprint(&sb, v)
	}
	sb.WriteRune('m')
	return sb.String()
}
