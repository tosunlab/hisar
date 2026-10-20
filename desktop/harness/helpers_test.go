package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeOmpScript plays omp's plugin manager: it logs each call and keeps the linked plugin in a file shared by every fake
// omp of a test, so the tests can tell which omp ran.
const fakeOmpScript = `#!/bin/sh
printf '%%s %%s\n' "$0" "$*" >> '%[1]s'
case "$1 $2" in
"plugin list")
  if [ -f '%[2]s/hang' ]; then exec sleep 10; fi
  if [ -f '%[2]s/garbled' ]; then echo 'Loading plugins'; exit 0; fi
  if [ -f '%[2]s/linked' ]; then
    printf '{"npm":[{"name":"@tryhisar/omp-plugin","version":"0.1.0","enabled":%%s}],"marketplace":[]}' "$(cat '%[2]s/linked')"
  else
    printf '{"npm":[{"name":"omp-other-plugin","version":"1.0.0","enabled":true}],"marketplace":[]}'
  fi
  ;;
"plugin link")
  if [ ! -f "$3/package.json" ]; then echo "✘ Failed to link: Error: package.json not found at $3" >&2; exit 1; fi
  echo true > '%[2]s/linked'
  echo "✔ Linked @tryhisar/omp-plugin from $3"
  ;;
"plugin uninstall")
  if [ ! -f '%[2]s/linked' ]; then echo "✘ $3 is not installed" >&2; exit 1; fi
  rm '%[2]s/linked'
  echo "✔ Uninstalled $3"
  ;;
esac
`

type fakeOmpWorld struct {
	t              *testing.T
	homeDirectory  string
	userPath       []string
	stateDirectory string
	logPath        string
}

func newFakeOmpWorld(t *testing.T) *fakeOmpWorld {
	t.Helper()
	root := t.TempDir()
	world := &fakeOmpWorld{
		t:              t,
		homeDirectory:  filepath.Join(root, "home"),
		userPath:       []string{filepath.Join(root, "usr-bin"), "relative/bin", "/usr/bin", "/bin"},
		stateDirectory: filepath.Join(root, "state"),
		logPath:        filepath.Join(root, "calls.log"),
	}
	for _, directory := range []string{world.homeDirectory, world.userPath[0], world.stateDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return world
}

func (world *fakeOmpWorld) pathOnUserPath() string { return filepath.Join(world.userPath[0], "omp") }
func (world *fakeOmpWorld) pathInHomeBin() string {
	return filepath.Join(world.homeDirectory, "bin", "omp")
}
func (world *fakeOmpWorld) pathInTern() string {
	return filepath.Join(world.homeDirectory, "Library", "Application Support", "Tern", "omp", "bin", "omp")
}
func (world *fakeOmpWorld) pathInBun() string {
	return filepath.Join(world.homeDirectory, ".bun", "bin", "omp")
}

func (world *fakeOmpWorld) installOmp(path string) {
	world.t.Helper()
	world.writeFile(path, fmt.Sprintf(fakeOmpScript, world.logPath, world.stateDirectory), 0o755)
}

func (world *fakeOmpWorld) writeFile(path, content string, mode os.FileMode) {
	world.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		world.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		world.t.Fatal(err)
	}
}

func (world *fakeOmpWorld) setState(name, content string) {
	world.t.Helper()
	world.writeFile(filepath.Join(world.stateDirectory, name), content, 0o644)
}

func (world *fakeOmpWorld) newService(pluginPath string) *Service {
	return NewService(Config{
		HomeDirectory: world.homeDirectory,
		GetUserPath:   func() []string { return world.userPath },
		OmpPluginPath: pluginPath,
	})
}

// newPlugin returns a plugin folder with a package.json, like Hisar.app's Contents/Resources/omp-plugin.
func (world *fakeOmpWorld) newPlugin() string {
	world.t.Helper()
	pluginPath := filepath.Join(world.t.TempDir(), "omp-plugin")
	world.writeFile(filepath.Join(pluginPath, "package.json"), `{"name":"@tryhisar/omp-plugin"}`, 0o644)
	return pluginPath
}

// readCalls returns each omp run as "<omp path> <arguments>".
func (world *fakeOmpWorld) readCalls() []string {
	world.t.Helper()
	content, err := os.ReadFile(world.logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		world.t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
}
