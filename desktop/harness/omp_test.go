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

func ompInTern(world *fakeWorld) string {
	return world.pathInHome("Library", "Application Support", "Tern", "omp", "bin", "omp")
}

func ompInBun(world *fakeWorld) string { return world.pathInHome(".bun", "bin", "omp") }

func TestFindOmpInTernFirstWhenNotOnPath(t *testing.T) {
	type location func(*fakeWorld) string
	onUserPath := func(world *fakeWorld) string { return world.pathOnUserPath("omp") }
	inHomeBin := func(world *fakeWorld) string { return world.pathInHome("bin", "omp") }
	inTern := ompInTern
	inBun := ompInBun
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
			world := newFakeWorld(t)
			for _, path := range test.installed {
				world.installOmp(path(world))
			}
			if agent := getAgent(t, world.newService(""), "omp"); agent.State != StateAvailable {
				t.Fatalf("omp = %+v; want available", agent)
			}
			if calls := world.readCalls(); len(calls) != 1 || calls[0] != test.want(world)+" plugin list --json" {
				t.Fatalf("omp calls = %q; want one plugin list by %s", calls, test.want(world))
			}
		})
	}
}

func TestOmpThatCannotRunIsNotInstalled(t *testing.T) {
	world := newFakeWorld(t)
	world.writeFile(world.pathInHome("bin", "omp"), "#!/bin/sh\n", 0o644)
	if err := os.MkdirAll(world.pathOnUserPath("omp"), 0o755); err != nil {
		t.Fatal(err)
	}
	service := world.newService(world.newPlugin())
	if agent := getAgent(t, service, "omp"); agent.State != StateNotInstalled {
		t.Fatalf("omp = %+v; want not-installed", agent)
	}
	if _, err := service.Connect("omp"); err == nil || err.Error() != "omp is not installed" {
		t.Fatalf("Connect() error = %v; want omp is not installed", err)
	}
	if agent, err := service.Disconnect("omp"); err != nil || agent.State != StateNotInstalled {
		t.Fatalf("Disconnect() = %+v, %v; want not-installed", agent, err)
	}
}

func TestFindOmpInstalledByBunRunsItWithBun(t *testing.T) {
	world := newFakeWorld(t)
	bunDirectory := filepath.Dir(ompInBun(world))
	world.writeFile(filepath.Join(bunDirectory, "bun"), "#!/bin/sh\nexec /bin/sh \"$@\"\n", 0o755)
	world.installOmp(filepath.Join(bunDirectory, "omp.js"))
	script, err := os.ReadFile(filepath.Join(bunDirectory, "omp.js"))
	if err != nil {
		t.Fatal(err)
	}
	world.writeFile(filepath.Join(bunDirectory, "omp.js"), strings.Replace(string(script), "#!/bin/sh", "#!/usr/bin/env bun", 1), 0o755)
	if err := os.Symlink("omp.js", ompInBun(world)); err != nil {
		t.Fatal(err)
	}
	if agent := getAgent(t, world.newService(""), "omp"); agent.State != StateAvailable {
		t.Fatalf("omp = %+v; want available", agent)
	}
}

func TestStateReadsThePluginList(t *testing.T) {
	for _, test := range []struct {
		name      string
		file      string
		content   string
		want      State
		wantError string
	}{
		{name: "only other plugins", want: StateAvailable},
		{name: "Hisar linked", file: "linked", content: "true", want: StateConnected},
		{name: "Hisar turned off in omp", file: "linked", content: "false", want: StateAvailable},
		{name: "a list that is not JSON", file: "garbled", want: StateUnknown, wantError: "omp listed its plugins in a form Hisar cannot read"},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := newFakeWorld(t)
			world.installOmp(ompInTern(world))
			if test.file != "" {
				world.setState(test.file, test.content)
			}
			want := Agent{ID: "omp", Name: "omp", State: test.want, Detail: test.wantError}
			if agent := getAgent(t, world.newService(""), "omp"); agent != want {
				t.Fatalf("omp = %+v; want %+v", agent, want)
			}
		})
	}
}

func TestConnectLinksTheBundledPlugin(t *testing.T) {
	world := newFakeWorld(t)
	world.installOmp(ompInTern(world))
	pluginPath := world.newPlugin()
	agent, err := world.newService(pluginPath).Connect("omp")
	if err != nil || agent.State != StateConnected {
		t.Fatalf("Connect() = %+v, %v; want connected", agent, err)
	}
	want := []string{ompInTern(world) + " plugin link " + pluginPath, ompInTern(world) + " plugin list --json"}
	if calls := world.readCalls(); !slices.Equal(calls, want) {
		t.Fatalf("omp calls = %q; want %q", calls, want)
	}
}

func TestConnectWithoutTheBundledPluginRunsNoOmp(t *testing.T) {
	world := newFakeWorld(t)
	world.installOmp(ompInTern(world))
	if _, err := world.newService("").Connect("omp"); err == nil || err.Error() != "this copy of Hisar does not include the omp plugin" {
		t.Fatalf("Connect() error = %v", err)
	}
	if calls := world.readCalls(); len(calls) != 0 {
		t.Fatalf("omp calls = %q; want none", calls)
	}
}

func TestConnectShowsWhyOmpRefused(t *testing.T) {
	world := newFakeWorld(t)
	world.installOmp(ompInTern(world))
	missingPath := filepath.Join(t.TempDir(), "omp-plugin")
	_, err := world.newService(missingPath).Connect("omp")
	want := "omp could not connect to Hisar: Failed to link: package.json not found at " + missingPath
	if err == nil || err.Error() != want {
		t.Fatalf("Connect() error = %v; want %q", err, want)
	}
}

func TestDisconnectUninstallsThePlugin(t *testing.T) {
	world := newFakeWorld(t)
	world.installOmp(ompInTern(world))
	service := world.newService(world.newPlugin())
	if _, err := service.Connect("omp"); err != nil {
		t.Fatal(err)
	}
	agent, err := service.Disconnect("omp")
	if err != nil || agent.State != StateAvailable {
		t.Fatalf("Disconnect() = %+v, %v; want available", agent, err)
	}
	if calls := world.readCalls(); !slices.Contains(calls, ompInTern(world)+" plugin uninstall @tryhisar/omp-plugin") {
		t.Fatalf("omp calls = %q; want plugin uninstall @tryhisar/omp-plugin", calls)
	}
	if agent, err := service.Disconnect("omp"); err != nil || agent.State != StateAvailable {
		t.Fatalf("second Disconnect() = %+v, %v; want available", agent, err)
	}
}

func TestOmpThatHangsTimesOut(t *testing.T) {
	world := newFakeWorld(t)
	world.installOmp(ompInTern(world))
	world.setState("hang", "")
	service := world.newService("")
	service.timeout = 500 * time.Millisecond
	started := time.Now()
	if agent := getAgent(t, service, "omp"); agent.Detail != "omp could not list its plugins: omp did not answer within 500ms" {
		t.Fatalf("omp = %+v", agent)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("ListAgents() took %s", elapsed)
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

	linuxPrefix := filepath.Join(root, "usr")
	write(filepath.Join(linuxPrefix, "share", "hisar", "omp-plugin", "package.json"))
	if got := FindOmpPlugin(filepath.Join(linuxPrefix, "bin")); got != filepath.Join(linuxPrefix, "share", "hisar", "omp-plugin") {
		t.Fatalf("FindOmpPlugin(Linux package) = %q", got)
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
