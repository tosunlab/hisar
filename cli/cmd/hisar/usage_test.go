package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var isGoldenUpdated = flag.Bool("update", false, "rewrite the golden files in testdata")

// TestUsageGolden keeps the command list and its wording unchanged.
func TestUsageGolden(t *testing.T) {
	var output bytes.Buffer
	printUsage(&output)
	path := filepath.Join("testdata", "usage.golden")
	if *isGoldenUpdated {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, output.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	golden, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), golden) {
		t.Errorf("usage changed; run go test -run TestUsageGolden -update if intended\n got:\n%s\nwant:\n%s", output.Bytes(), golden)
	}
}
