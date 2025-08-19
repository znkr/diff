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

// Package config provides shared configuration mechanisms for packages this module.
//
// This package is an implementation detail, the configuration surface for users is provided via
// diff.Option.
package config

// Config collects all configurable parameters for comparison functions in this module.
type Config struct {
	// Context is the number of matches to include as a prefix and postfix for hunks returned.
	Context int

	// If set, comparison function will try to find an optimal diff irrespective of the cost. By
	// default, the comparison functions in this package limit the cost for large inputs with many
	// differences by applying heuristics that reduce the time complexity.
	Optimal bool

	// If set, textdiff will apply ident heuristics.
	IndentHeuristic bool

	// If set, internal/myers will apply the anchoring heuristic.
	AnchoringHeuristic bool
}

// Default is the default configuration.
var Default = Config{
	Context:            3,
	Optimal:            false,
	IndentHeuristic:    false,
	AnchoringHeuristic: false,
}

// Flag describes a single config entry. This is used to detect if configurations are being set
// that are not
type Flag int

const (
	Context Flag = 1 << iota
	Optimal
	IndentHeuristic
	AnchoringHeuristic
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
	if cfg.Optimal && cfg.AnchoringHeuristic {
		panic("Options diff.Optimal and diff.AnchoringHeuristic cannot be set at the same time")
	}
	return cfg
}

func printFlag(flag Flag) string {
	switch flag {
	case Context:
		return "diff.Context"
	case Optimal:
		return "diff.Optimal"
	case IndentHeuristic:
		return "textdiff.IndentHeuristic"
	case AnchoringHeuristic:
		return "diff.AnchoringHeuristic"
	default:
		panic("never reached")
	}
}
