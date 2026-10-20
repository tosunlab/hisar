package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestFindOmpInTernFirstWhenNotOnPath(t *testing.T) {
	type location func(*fakeOmpWorld) string
	onUserPath := (*fakeOmpWorld).pathOnUserPath
	inHomeBin := (*fakeOmpWorld).pathInHomeBin
	inTern := (*fakeOmpWorld).pathInTern
	inBun := (*fakeOmpWorld).pathInBun
	for _, test := range []struct {
		name      string
		installed []location
		want      location
	}{
		{"the login shell's PATH first", []location{inBun, inTern, inHomeBin, onUserPath}, onUserPath},
		{"then omp's installer", []location{inBun, inTern, inHomeBin}, inHomeBin},
		{"then Tern", []location{inBun, inTern}, inTern},
		{"then Bun", []location{inBun}, inBun},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := newFakeOmpWorld(t)
			for _, path := range test.installed {
				world.installOmp(path(world))
			}
			state, err := world.newService("").GetState()
			if err != nil || state != OmpAvailable {
				t.Fatalf("GetState() = %q, %v; want available", state, err)
			}
			if calls := world.readCalls(); len(calls) != 1 || calls[0] != test.want(world)+" plugin list --json" {
				t.Fatalf("omp calls = %q; want one plugin list by %s", calls, test.want(world))
			}
		})
	}
}

func TestOmpThatCannotRunIsNotInstalled(t *testing.T) {
	world := newFakeOmpWorld(t)
	world.writeFile(world.pathInHomeBin(), "#!/bin/sh\n", 0o644)
	if err := os.MkdirAll(world.pathOnUserPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	service := world.newService(world.newPlugin())
	if state, err := service.GetState(); err != nil || state != OmpNotInstalled {
		t.Fatalf("GetState() = %q, %v; want not-installed", state, err)
	}
	if _, err := service.Connect(); err == nil || err.Error() != "omp is not installed" {
		t.Fatalf("Connect() error = %v; want omp is not installed", err)
	}
	if state, err := service.Disconnect(); err != nil || state != OmpNotInstalled {
		t.Fatalf("Disconnect() = %q, %v; want not-installed", state, err)
	}
}

func TestFindOmpInstalledByBunRunsItWithBun(t *testing.T) {
	world := newFakeOmpWorld(t)
	bunDirectory := filepath.Dir(world.pathInBun())
	world.writeFile(filepath.Join(bunDirectory, "bun"), "#!/bin/sh\nexec /bin/sh \"$@\"\n", 0o755)
	world.installOmp(filepath.Join(bunDirectory, "omp.js"))
	script, err := os.ReadFile(filepath.Join(bunDirectory, "omp.js"))
	if err != nil {
		t.Fatal(err)
	}
	world.writeFile(filepath.Join(bunDirectory, "omp.js"), strings.Replace(string(script), "#!/bin/sh", "#!/usr/bin/env bun", 1), 0o755)
	if err := os.Symlink("omp.js", world.pathInBun()); err != nil {
		t.Fatal(err)
	}
	if state, err := world.newService("").GetState(); err != nil || state != OmpAvailable {
		t.Fatalf("GetState() = %q, %v; want available", state, err)
	}
}

func TestStateReadsThePluginList(t *testing.T) {
	for _, test := range []struct {
		name      string
		file      string
		content   string
		want      OmpState
		wantError string
	}{
		{name: "only other plugins", want: OmpAvailable},
		{name: "Hisar linked", file: "linked", content: "true", want: OmpConnected},
		{name: "Hisar turned off in omp", file: "linked", content: "false", want: OmpAvailable},
		{name: "a list that is not JSON", file: "garbled", wantError: "omp listed its plugins in a form Hisar cannot read"},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := newFakeOmpWorld(t)
			world.installOmp(world.pathInTern())
			if test.file != "" {
				world.setState(test.file, test.content)
			}
			state, err := world.newService("").GetState()
			if test.wantError != "" {
				if err == nil || err.Error() != test.wantError {
					t.Fatalf("GetState() error = %v; want %q", err, test.wantError)
				}
				return
			}
			if err != nil || state != test.want {
				t.Fatalf("GetState() = %q, %v; want %q", state, err, test.want)
			}
		})
	}
}

func TestConnectLinksTheBundledPlugin(t *testing.T) {
	world := newFakeOmpWorld(t)
	world.installOmp(world.pathInTern())
	pluginPath := world.newPlugin()
	state, err := world.newService(pluginPath).Connect()
	if err != nil || state != OmpConnected {
		t.Fatalf("Connect() = %q, %v; want connected", state, err)
	}
	want := []string{world.pathInTern() + " plugin link " + pluginPath, world.pathInTern() + " plugin list --json"}
	if calls := world.readCalls(); !slices.Equal(calls, want) {
		t.Fatalf("omp calls = %q; want %q", calls, want)
	}
}

func TestConnectWithoutTheBundledPluginRunsNoOmp(t *testing.T) {
	world := newFakeOmpWorld(t)
	world.installOmp(world.pathInTern())
	if _, err := world.newService("").Connect(); err == nil || err.Error() != "this copy of Hisar does not include the omp plugin" {
		t.Fatalf("Connect() error = %v", err)
	}
	if calls := world.readCalls(); len(calls) != 0 {
		t.Fatalf("omp calls = %q; want none", calls)
	}
}

func TestConnectShowsWhyOmpRefused(t *testing.T) {
	world := newFakeOmpWorld(t)
	world.installOmp(world.pathInTern())
	missingPath := filepath.Join(t.TempDir(), "omp-plugin")
	_, err := world.newService(missingPath).Connect()
	want := "omp could not connect to Hisar: Failed to link: package.json not found at " + missingPath
	if err == nil || err.Error() != want {
		t.Fatalf("Connect() error = %v; want %q", err, want)
	}
}

func TestDisconnectUninstallsThePlugin(t *testing.T) {
	world := newFakeOmpWorld(t)
	world.installOmp(world.pathInTern())
	service := world.newService(world.newPlugin())
	if _, err := service.Connect(); err != nil {
		t.Fatal(err)
	}
	state, err := service.Disconnect()
	if err != nil || state != OmpAvailable {
		t.Fatalf("Disconnect() = %q, %v; want available", state, err)
	}
	if calls := world.readCalls(); !slices.Contains(calls, world.pathInTern()+" plugin uninstall @tryhisar/omp-plugin") {
		t.Fatalf("omp calls = %q; want plugin uninstall @tryhisar/omp-plugin", calls)
	}
	if state, err := service.Disconnect(); err != nil || state != OmpAvailable {
		t.Fatalf("second Disconnect() = %q, %v; want available", state, err)
	}
}

func TestOmpThatHangsTimesOut(t *testing.T) {
	world := newFakeOmpWorld(t)
	world.installOmp(world.pathInTern())
	world.setState("hang", "")
	service := world.newService("")
	service.timeout = 500 * time.Millisecond
	started := time.Now()
	_, err := service.GetState()
	if err == nil || err.Error() != "omp could not list its plugins: omp did not answer within 500ms" {
		t.Fatalf("GetState() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("GetState() took %s", elapsed)
	}
}

func TestFindOmpPlugin(t *testing.T) {
	root := t.TempDir()
	write := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	appContents := filepath.Join(root, "Applications", "Hisar.app", "Contents")
	write(filepath.Join(appContents, "Resources", "omp-plugin", "package.json"))
	if got := FindOmpPlugin(filepath.Join(appContents, "MacOS")); got != filepath.Join(appContents, "Resources", "omp-plugin") {
		t.Fatalf("FindOmpPlugin(Hisar.app) = %q", got)
	}

	repository := filepath.Join(root, "hisar")
	write(filepath.Join(repository, "omp", "package.json"))
	if got := FindOmpPlugin(filepath.Join(repository, "desktop", "bin")); got != filepath.Join(repository, "omp") {
		t.Fatalf("FindOmpPlugin(development build) = %q", got)
	}

	if got := FindOmpPlugin(filepath.Join(root, "elsewhere", "bin")); got != "" {
		t.Fatalf("FindOmpPlugin(no plugin) = %q; want none", got)
	}
}

func TestPluginNameIsTheNameInThePackage(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "omp", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Name != ompPluginName {
		t.Fatalf("omp/package.json names %q; Disconnect uninstalls %q", manifest.Name, ompPluginName)
	}
}
