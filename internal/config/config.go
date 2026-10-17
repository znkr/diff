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

// Package config provides shared configuration mechanisms for packages this
// module.
//
// This package is an implementation detail, the configuration surface for users
// is provided via diff.Option.
package config

// Mode describes the mode of the diff algorithm.
type Mode int

const (
	// Limit the cost for large inputs with many differences by applying
	// heuristics that reduce the time complexity at the cost of non-minimal
	// diffs.
	ModeDefault Mode = iota

	// Find a minimal diff irrespective of the cost.
	ModeMinimal

	// Find a diff as fast as possible.
	ModeFast
)

// Config collects all configurable parameters for comparison functions in this
// module.
type Config struct {
	// Context is the number of matches to include as a prefix and postfix for
	// hunks returned.
	Context int

	// Diff algorithm mode.
	Mode Mode

	// If set, textdiff will apply ident heuristics.
	IndentHeuristic bool

	// Unified diff output configuration.
	Unified *Unified

	// If set, internal/myers will always use the anchoring heuristic. This
	// configuration is not exposed via an option API, it's main use is for
	// testing.
	ForceAnchoringHeuristic bool
}

// Unified configures the output of textdiff.Unified, textdiff.AppendUnified
// and textdiff.WriteUnified.
type Unified struct {
	// If not nil, the output starts with the file header "--- X" and "+++ Y".
	Header *UnifiedHeaderConfig

	// Color escape sequences. The zero value disables colors.
	Color ColorConfig
}

// UnifiedHeaderConfig holds the labels of the file header of a unified diff.
type UnifiedHeaderConfig struct {
	X, Y string
}

type ColorConfig struct {
	Reset                 string
	FileHeader            string
	HunkHeader            string
	Match, Delete, Insert string
}

// Default is the default configuration.
var Default = Config{
	Context:                 3,
	Mode:                    ModeDefault,
	IndentHeuristic:         false,
	ForceAnchoringHeuristic: false,
}

// Flag describes a single config entry. This is used to detect if
// configurations are being set that are not
type Flag int

const (
	Context Flag = 1 << iota
	Minimal
	Fast
	IndentHeuristic
	TerminalColors
	UnifiedHeader
)

// Option is the mechanism used to expose the configuration to users.
type Option func(*Config) Flag

// FromOptions creates a configuration from a set of options.
func FromOptions(opts []Option, allowed Flag) Config {
	cfg := Default
	for _, opt := range opts {
		flag := opt(&cfg)
		if flag & ^allowed != 0 {
			panic("Option " + printFlag(flag) + " not allowed here")
		}
	}
	if cfg.Mode != ModeDefault && cfg.ForceAnchoringHeuristic {
		panic("ForceAnchoringHeuristic may only be set for ModeDefault")
	}
	return cfg
}

func printFlag(flag Flag) string {
	switch flag {
	case Context:
		return "diff.Context"
	case Minimal:
		return "diff.Minimal"
	case Fast:
		return "diff.Fast"
	case IndentHeuristic:
		return "textdiff.IndentHeuristic"
	case TerminalColors:
		return "textdiff.TerminalColors"
	case UnifiedHeader:
		return "textdiff.UnifiedHeader"
	default:
		panic("never reached")
	}
}
