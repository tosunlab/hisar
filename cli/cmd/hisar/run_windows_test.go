package main

import (
	"flag"
	"io"
)

func createTestFlagSet() *flag.FlagSet {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func runSignalHelper(string) int { return 99 }
