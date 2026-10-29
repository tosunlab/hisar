package appinfo

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maximumIconSize keeps a theme's oversized file out of the approval window.
const maximumIconSize = 512 << 10

// desktopApps names the Linux app behind a process from its desktop entry: the nearest process in the chain whose
// executable an entry starts, or else the systemd unit the desktop started the process in ("app-<desktop ID>-<n>.scope",
// in /proc/<pid>/cgroup). Desktop entries name an app but prove nothing about it, so what it finds is shown and never
// trusted as an approval's identity.
type desktopApps struct {
	// procDirectory is /proc; dataDirectories are $XDG_DATA_HOME then $XDG_DATA_DIRS, each holding applications/ and
	// icons/.
	procDirectory   string
	dataDirectories []string
	lookPath        func(file string) (string, error)

	mutex      sync.Mutex
	entries    []desktopEntry
	readAt     time.Time
	iconsByKey map[string]string
}

type desktopEntry struct {
	id       string
	path     string
	name     string
	icon     string
	execPath string
}

// newSystemDesktopApps reads the session's XDG directories, with the defaults the XDG Base Directory specification
// gives.
func newSystemDesktopApps() *desktopApps {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dataHome = filepath.Join(home, ".local", "share")
		}
	}
	dataDirectories := []string{}
	if dataHome != "" {
		dataDirectories = append(dataDirectories, dataHome)
	}
	systemDirectories := os.Getenv("XDG_DATA_DIRS")
	if systemDirectories == "" {
		systemDirectories = "/usr/local/share:/usr/share"
	}
	for _, directory := range filepath.SplitList(systemDirectories) {
		if filepath.IsAbs(directory) {
			dataDirectories = append(dataDirectories, directory)
		}
	}
	return &desktopApps{procDirectory: "/proc", dataDirectories: dataDirectories, lookPath: exec.LookPath}
}

// find returns the app and how many processes of the chain run before it: the chain the dialog lists.
func (apps *desktopApps) find(processes []Process) (App, int, bool) {
	entries := apps.listEntries()
	for index, process := range processes {
		if entry, isFound := findEntryByExecutable(entries, process.Path); isFound {
			return apps.describe(entry), index, true
		}
	}
	if len(processes) > 0 {
		if entry, isFound := apps.findEntryByUnit(entries, processes[0].PID); isFound {
			return apps.describe(entry), len(processes), true
		}
	}
	return App{}, 0, false
}

// findEntryByExecutable matches the one entry that starts path; an executable two apps share names neither.
func findEntryByExecutable(entries []desktopEntry, path string) (desktopEntry, bool) {
	if path == "" {
		return desktopEntry{}, false
	}
	var found desktopEntry
	isFound := false
	for _, entry := range entries {
		if entry.execPath != path {
			continue
		}
		if isFound && entry.name != found.name {
			return desktopEntry{}, false
		}
		found, isFound = entry, true
	}
	return found, isFound
}

// findEntryByUnit reads the systemd units pid runs in, innermost first, and takes the first "app-" unit that names an
// entry. The unit is app[-<launcher>]-<desktop ID>[-<n>].scope, app[-<launcher>]-<desktop ID>[@<n>].service or
// app-<desktop ID>.slice, with each "-" of the ID written as \x2d, so each "-"-separated field is tried as an ID.
func (apps *desktopApps) findEntryByUnit(entries []desktopEntry, pid int) (desktopEntry, bool) {
	cgroup, err := os.ReadFile(filepath.Join(apps.procDirectory, strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return desktopEntry{}, false
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(cgroup)), "\n") {
		// hierarchy-ID:controllers:path, the one "0::" line on cgroup v2.
		fields := strings.SplitN(line, ":", 3)
		if len(fields) != 3 {
			continue
		}
		units := strings.Split(fields[2], "/")
		for _, unit := range slices.Backward(units) {
			name, isApp := strings.CutPrefix(unit, "app-")
			if !isApp {
				continue
			}
			for _, suffix := range []string{".scope", ".service", ".slice"} {
				name = strings.TrimSuffix(name, suffix)
			}
			name, _, _ = strings.Cut(name, "@")
			for _, field := range slices.Backward(strings.Split(name, "-")) {
				id := strings.ReplaceAll(field, `\x2d`, "-")
				if index := slices.IndexFunc(entries, func(entry desktopEntry) bool { return entry.id == id }); index >= 0 {
					return entries[index], true
				}
			}
		}
	}
	return desktopEntry{}, false
}

func (apps *desktopApps) describe(entry desktopEntry) App {
	return App{Name: entry.name, IconURL: apps.getIconURL(entry.icon)}
}

// listEntries reads every application's desktop entry again once the applications directories change. An entry in an
// earlier data directory hides one with the same ID in a later one, as the specification says.
func (apps *desktopApps) listEntries() []desktopEntry {
	latest := time.Time{}
	for _, directory := range apps.dataDirectories {
		if info, err := os.Stat(filepath.Join(directory, "applications")); err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	apps.mutex.Lock()
	defer apps.mutex.Unlock()
	if apps.entries != nil && apps.readAt.Equal(latest) {
		return apps.entries
	}
	entries := []desktopEntry{}
	isSeenByID := map[string]bool{}
	for _, directory := range apps.dataDirectories {
		applications := filepath.Join(directory, "applications")
		filepath.WalkDir(applications, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".desktop") {
				return nil
			}
			relative, _ := filepath.Rel(applications, path)
			id := strings.TrimSuffix(strings.ReplaceAll(relative, string(filepath.Separator), "-"), ".desktop")
			if isSeenByID[id] {
				return nil
			}
			isSeenByID[id] = true
			if parsed, isApp := apps.readEntry(path); isApp {
				parsed.id, parsed.path = id, path
				entries = append(entries, parsed)
			}
			return nil
		})
	}
	apps.entries, apps.readAt, apps.iconsByKey = entries, latest, map[string]string{}
	return entries
}

// readEntry reads an application's [Desktop Entry] group. One hidden, for a terminal, or without a name is no app to
// name.
func (apps *desktopApps) readEntry(path string) (desktopEntry, bool) {
	file, err := os.Open(path)
	if err != nil {
		return desktopEntry{}, false
	}
	defer file.Close()
	values := map[string]string{}
	isInGroup := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			isInGroup = line == "[Desktop Entry]"
			continue
		}
		if key, value, isFound := strings.Cut(line, "="); isInGroup && isFound {
			if key = strings.TrimSpace(key); values[key] == "" {
				values[key] = strings.TrimSpace(value)
			}
		}
	}
	if values["Type"] != "Application" || values["Name"] == "" || values["Hidden"] == "true" || values["Terminal"] == "true" {
		return desktopEntry{}, false
	}
	return desktopEntry{name: values["Name"], icon: values["Icon"], execPath: apps.resolveExec(values["Exec"])}, true
}

// launcherPrograms start another program, so an entry they begin with names whatever they run, not them.
var launcherPrograms = []string{"env", "flatpak", "snap", "sh", "bash", "dash", "zsh", "python", "python3", "perl", "java", "gio", "xdg-open", "kioclient", "kioclient5"}

// resolveExec is the absolute executable an Exec line starts, or "" when the line runs a launcher or passes the
// program anything but options and field codes (git gui), where the executable does not name the app.
func (apps *desktopApps) resolveExec(line string) string {
	arguments := splitExec(line)
	if len(arguments) == 0 || slices.Contains(launcherPrograms, filepath.Base(arguments[0])) {
		return ""
	}
	for _, argument := range arguments[1:] {
		if !strings.HasPrefix(argument, "-") && !strings.HasPrefix(argument, "%") {
			return ""
		}
	}
	program := arguments[0]
	if !filepath.IsAbs(program) {
		resolved, err := apps.lookPath(program)
		if err != nil {
			return ""
		}
		program = resolved
	}
	if resolved, err := filepath.EvalSymlinks(program); err == nil {
		program = resolved
	}
	return program
}

// splitExec splits an Exec value into arguments, with double quotes and backslash escapes as the specification has them.
func splitExec(line string) []string {
	arguments := []string{}
	var argument strings.Builder
	isQuoted, hasArgument := false, false
	for index := 0; index < len(line); index++ {
		character := line[index]
		switch {
		case character == '\\' && isQuoted && index+1 < len(line):
			index++
			argument.WriteByte(line[index])
		case character == '"':
			isQuoted, hasArgument = !isQuoted, true
		case character == ' ' && !isQuoted:
			if hasArgument {
				arguments = append(arguments, argument.String())
				argument.Reset()
				hasArgument = false
			}
		default:
			argument.WriteByte(character)
			hasArgument = true
		}
	}
	if hasArgument {
		arguments = append(arguments, argument.String())
	}
	return arguments
}

// getIconURL finds an entry's icon as a data URL: an absolute path, or a name in the icon themes and pixmaps of every
// data directory, hicolor first. The largest PNG of at least DialogIconSize wins, then an SVG, then the largest PNG.
func (apps *desktopApps) getIconURL(icon string) string {
	if icon == "" {
		return ""
	}
	apps.mutex.Lock()
	iconURL, isCached := apps.iconsByKey[icon]
	apps.mutex.Unlock()
	if isCached {
		return iconURL
	}
	path := icon
	if !filepath.IsAbs(icon) {
		path = apps.findIconFile(icon)
	}
	iconURL = readIconURL(path)
	apps.mutex.Lock()
	if apps.iconsByKey == nil {
		apps.iconsByKey = map[string]string{}
	}
	apps.iconsByKey[icon] = iconURL
	apps.mutex.Unlock()
	return iconURL
}

func (apps *desktopApps) findIconFile(name string) string {
	bestPNG, bestPNGSize, svg := "", 0, ""
	consider := func(path string, size int) {
		switch filepath.Ext(path) {
		case ".svg":
			if svg == "" {
				svg = path
			}
		case ".png":
			isBetter := bestPNG == "" ||
				(size >= DialogIconSize && (bestPNGSize < DialogIconSize || size < bestPNGSize)) ||
				(size < DialogIconSize && bestPNGSize < DialogIconSize && size > bestPNGSize)
			if isBetter {
				bestPNG, bestPNGSize = path, size
			}
		}
	}
	for _, directory := range apps.dataDirectories {
		iconsDirectory := filepath.Join(directory, "icons")
		themes, _ := os.ReadDir(iconsDirectory)
		themeNames := []string{}
		for _, theme := range themes {
			if theme.IsDir() {
				themeNames = append(themeNames, theme.Name())
			}
		}
		slices.SortStableFunc(themeNames, func(first, second string) int {
			return boolToInt(second == "hicolor") - boolToInt(first == "hicolor")
		})
		for _, theme := range themeNames {
			themeDirectory := filepath.Join(iconsDirectory, theme)
			filepath.WalkDir(themeDirectory, func(path string, entry fs.DirEntry, err error) error {
				if err == nil && !entry.IsDir() && strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())) == name {
					relative, _ := filepath.Rel(themeDirectory, path)
					consider(path, getIconDirectorySize(relative))
				}
				return nil
			})
		}
		for _, extension := range []string{".png", ".svg"} {
			if path := filepath.Join(directory, "pixmaps", name+extension); isRegularFile(path) {
				consider(path, 0)
			}
		}
	}
	if bestPNG != "" && bestPNGSize >= DialogIconSize {
		return bestPNG
	}
	if svg != "" {
		return svg
	}
	return bestPNG
}

// getIconDirectorySize reads an icon's size from its path in the theme: 128x128 or 128x128@2 (hicolor, Adwaita) or 48
// (Breeze), 0 for none.
func getIconDirectorySize(path string) int {
	for _, part := range strings.Split(filepath.Dir(path), string(filepath.Separator)) {
		part, _, _ = strings.Cut(part, "@")
		width, _, _ := strings.Cut(part, "x")
		if size, err := strconv.Atoi(width); err == nil {
			return size
		}
	}
	return 0
}

func readIconURL(path string) string {
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maximumIconSize {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	case filepath.Ext(path) == ".svg":
		return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(data)
	}
	return ""
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
