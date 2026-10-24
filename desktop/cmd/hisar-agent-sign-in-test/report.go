package main

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type report struct {
	mutex            sync.Mutex
	CuaDriverVersion string   `json:"cuaDriverVersion"`
	HermesVersion    string   `json:"hermesVersion,omitempty"`
	Results          []result `json:"results"`
}

type result struct {
	Layer    string        `json:"layer"`
	Browser  string        `json:"browser"`
	Scenario string        `json:"scenario"`
	Passed   bool          `json:"passed"`
	Detail   string        `json:"detail"`
	Elapsed  time.Duration `json:"elapsed"`
	Retries  int           `json:"pageRefusalRetries"`
}

func (found *report) add(entry result) {
	found.mutex.Lock()
	defer found.mutex.Unlock()
	found.Results = append(found.Results, entry)
	verdict := "PASS"
	if !entry.Passed {
		verdict = "FAIL"
	}
	fmt.Printf("%s  %-6s %-14s %-28s retries=%d %s\n", verdict, entry.Layer, entry.Browser, entry.Scenario, entry.Retries, entry.Detail)
}

func (found *report) print(writer io.Writer) int {
	found.mutex.Lock()
	defer found.mutex.Unlock()
	fmt.Fprintf(writer, "\ncua-driver: %s\n", found.CuaDriverVersion)
	if found.HermesVersion != "" {
		fmt.Fprintf(writer, "Hermes: %s\n", found.HermesVersion)
	}
	failed := 0
	for _, entry := range found.Results {
		verdict := "PASS"
		if !entry.Passed {
			verdict = "FAIL"
			failed++
		}
		fmt.Fprintf(writer, "| %s | %s | %s | %s | %d | %s |\n", entry.Layer, entry.Browser, entry.Scenario, verdict, entry.Retries, strings.ReplaceAll(entry.Detail, "|", "/"))
	}
	fmt.Fprintf(writer, "%d scenarios, %d failed\n", len(found.Results), failed)
	return failed
}
