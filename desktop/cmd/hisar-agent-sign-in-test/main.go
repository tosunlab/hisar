// Command hisar-agent-sign-in-test exercises website sign-in with an isolated Hisar
// account and browser profiles through native window controls and the MCP server.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.Ltime)
	if len(os.Args) > 2 && os.Args[1] == "relay" {
		relay(os.Args[2:])
		return
	}
	if len(os.Args) < 2 {
		log.Fatal("usage: hisar-agent-sign-in-test run|scan …")
	}
	switch os.Args[1] {
	case "run":
		runCommand(os.Args[2:])
	case "scan":
		scanCommand(os.Args[2:])
	default:
		log.Fatalf("unknown command %s", os.Args[1])
	}
}

type browserFlags []browserApp

func (browsers *browserFlags) String() string { return fmt.Sprint(*browsers) }

func (browsers *browserFlags) Set(value string) error {
	name, app, isFound := strings.Cut(value, "=")
	if !isFound {
		return fmt.Errorf("want name=/path/to/Browser.app, got %q", value)
	}
	*browsers = append(*browsers, browserApp{Name: name, App: app})
	return nil
}

type run struct {
	directory   string
	hisarPath   string
	harnessPath string
	sitePath    string
	ocrPath     string
	ax          axTool
	cua         cuaDriver
	desktop     *desktopApp
	paths       desktopPaths
	account     account
	canaries    canaries
	report      *report
	started     time.Time
	retries     atomic.Int64
	mutex       sync.Mutex
	cleanups    []func()
}

func (current *run) onExit(cleanup func()) {
	current.mutex.Lock()
	defer current.mutex.Unlock()
	current.cleanups = append(current.cleanups, cleanup)
}

func (current *run) cleanUp() {
	current.mutex.Lock()
	cleanups := current.cleanups
	current.cleanups = nil
	current.mutex.Unlock()
	for index := len(cleanups) - 1; index >= 0; index-- {
		cleanups[index]()
	}
}

func (current *run) fatal(err error) {
	log.Printf("stopping the run: %v", err)
	current.cleanUp()
	os.Exit(2)
}

func runCommand(arguments []string) {
	flags := flag.NewFlagSet("run", flag.ExitOnError)
	directory := flags.String("dir", "", "the run's private folder")
	invite := flags.String("invite", "", "an invite link of the local hisar-server for the throwaway account")
	hisarPath := flags.String("hisar", "", "the hisar command-line tool")
	desktopPath := flags.String("desktop", "", "the development Hisar executable")
	sitePath := flags.String("site", "", "hisar-sign-in-site-fixture")
	axPath := flags.String("ax", "", "scripts/agent-sign-in/ax.swift, built")
	ocrPath := flags.String("ocr", "", "scripts/agent-sign-in/ocr.swift, built")
	cuaPath := flags.String("cua-driver", "", "cua-driver")
	unsupported := flags.String("unsupported-browser", "", "a Chromium app Hisar does not support, such as Chrome for Testing")
	scenarioFilter := flags.String("scenarios", "", "comma-separated scenario names to run (default: all)")
	var browsers browserFlags
	flags.Var(&browsers, "browser", "name=/path/to/Browser.app, repeatable")
	flags.Parse(arguments)
	if *directory == "" || *invite == "" || len(browsers) == 0 || driverSocket() == "" {
		log.Fatal("run needs -dir, -invite, -browser and the private driver socket")
	}
	if _, err := readDisplayLayout(); err != nil {
		log.Fatal(err)
	}
	driverOutput := &bytes.Buffer{}
	current := &run{
		directory: *directory, hisarPath: *hisarPath, sitePath: *sitePath, ax: axTool{path: *axPath},
		cua: cuaDriver{path: *cuaPath, transcript: driverOutput}, ocrPath: *ocrPath, report: &report{}, started: time.Now(),
	}
	current.paths = newDesktopPaths(*directory, *desktopPath)
	defer current.cleanUp()
	current.onExit(func() {
		os.WriteFile(filepath.Join(*directory, "logs", "cua-driver.log"), driverOutput.Bytes(), 0o600)
	})
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		current.fatal(errors.New("interrupted"))
	}()
	if err := current.prepareHarness(); err != nil {
		current.fatal(err)
	}
	only := splitList(*scenarioFilter)
	var hermes *hermesRuntime
	if os.Getenv("AGENT_SIGN_IN_LAYER") == "B" {
		var err error
		hermes, err = current.prepareHermes()
		if err != nil {
			current.fatal(err)
		}
		only = []string{"hermes-setup"}
	}
	runs := current.runLayerA(*invite, browsers, browserApp{Name: "Chrome for Testing", App: *unsupported}, only)
	if hermes != nil {
		if runs[0].browser == nil {
			current.fatal(errors.New("Layer B browser failed to launch"))
		}
		runs[0].runLayerB(hermes, splitList(*scenarioFilter))
	}
	current.cleanUp()
	current.report.CuaDriverVersion = current.cua.version()
	encoded, _ := json.MarshalIndent(current.report, "", "  ")
	os.WriteFile(filepath.Join(*directory, "report.json"), encoded, 0o600)
	if current.report.print(os.Stdout) > 0 {
		os.Exit(1)
	}
}

func splitList(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

func (current *run) prepareHarness() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	current.harnessPath = filepath.Join(current.directory, "agent", harnessName)
	if err := os.MkdirAll(filepath.Dir(current.harnessPath), 0o700); err != nil {
		return err
	}
	source, err := os.Open(self)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(current.harnessPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		return err
	}
	return target.Close()
}

func (current *run) connectAgent(transcript *lockedBuffer) (*agent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return connectAgent(ctx, current.harnessPath, current.hisarPath, current.paths.CLISocket, transcript)
}
