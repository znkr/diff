// diff is a small CLI to manually run the diffing implementations used for benchmarking.
package main

import (
	"flag"
	"fmt"
	"os"

	"golang.org/x/tools/txtar"
	"znkr.io/diff/internal/benchmarks"
)

type config struct {
	lib   string
	x, y  string
	txtar string
}

func main() {
	var cfg config
	flag.StringVar(&cfg.lib, "lib", "znkr", "library to use for diffing")
	flag.StringVar(&cfg.txtar, "txtar", "", "use testdata txtar file instead of two input files")
	flag.Parse()

	if cfg.txtar != "" {
		if flag.CommandLine.NArg() != 0 {
			fmt.Fprintf(os.Stderr, "error: usage: diff -txtar <file>\n")
			os.Exit(1)
		}
	} else {
		if flag.CommandLine.NArg() != 2 {
			fmt.Fprintf(os.Stderr, "error: usage: diff <x> <y>\n")
			os.Exit(1)
		}
		cfg.x = flag.CommandLine.Arg(0)
		cfg.y = flag.CommandLine.Arg(1)
	}

	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	var lib *benchmarks.Impl
	for _, l := range benchmarks.Impls {
		if l.Name == cfg.lib {
			lib = &l
		}
	}
	if lib == nil {
		return fmt.Errorf("lib not found %q", cfg.lib)
	}

	var x, y []byte
	if cfg.txtar != "" {
		ar, err := txtar.ParseFile(cfg.txtar)
		if err != nil {
			return err
		}
		for _, f := range ar.Files {
			switch f.Name {
			case "x":
				x = f.Data
			case "y":
				y = f.Data
			}
		}
	} else {
		var err error
		x, err = os.ReadFile(cfg.x)
		if err != nil {
			return err
		}
		y, err = os.ReadFile(cfg.y)
		if err != nil {
			return err
		}
	}

	out := lib.Diff(x, y)
	os.Stdout.Write(out)
	return nil
}
