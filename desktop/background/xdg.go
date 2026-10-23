package background

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const autostartFileName = BundleID + ".desktop"

// XDGAutostart is the login item on Linux desktops: a .desktop file in $XDG_CONFIG_HOME/autostart.
type XDGAutostart struct {
	ConfigDirectory string
	ExecutablePath  string
}

func NewXDGAutostart(homeDirectory, executablePath string) *XDGAutostart {
	configDirectory := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(configDirectory) {
		configDirectory = filepath.Join(homeDirectory, ".config")
	}
	return &XDGAutostart{ConfigDirectory: configDirectory, ExecutablePath: executablePath}
}

func (autostart *XDGAutostart) GetPath() string {
	return filepath.Join(autostart.ConfigDirectory, "autostart", autostartFileName)
}

func (autostart *XDGAutostart) GetState() (LoginItemState, error) {
	if autostart.ExecutablePath == "" {
		return LoginItemUnavailable, nil
	}
	file, err := os.Open(autostart.GetPath())
	if errors.Is(err, fs.ErrNotExist) {
		return LoginItemNotRegistered, nil
	}
	if err != nil {
		return LoginItemNotRegistered, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, isFound := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !isFound {
			continue
		}
		key, value = strings.TrimSpace(key), strings.ToLower(strings.TrimSpace(value))
		if (key == "Hidden" && value == "true") || (key == "X-GNOME-Autostart-enabled" && value == "false") {
			return LoginItemNotRegistered, nil
		}
	}
	return LoginItemEnabled, scanner.Err()
}

func (autostart *XDGAutostart) Register() error {
	if autostart.ExecutablePath == "" {
		return ErrUnavailable
	}
	if err := os.MkdirAll(filepath.Dir(autostart.GetPath()), 0o755); err != nil {
		return err
	}
	content := strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=Hisar",
		"Comment=Keeps the Hisar SSH agent and command-line tool available",
		"Exec=" + quoteExecArgument(autostart.ExecutablePath) + " " + HiddenArgument,
		"Icon=Hisar",
		"Terminal=false",
		"X-GNOME-Autostart-enabled=true",
		"",
	}, "\n")
	temporaryFile, err := os.CreateTemp(filepath.Dir(autostart.GetPath()), "."+filepath.Base(autostart.GetPath())+".*")
	if err != nil {
		return err
	}
	_, err = temporaryFile.WriteString(content)
	if err == nil {
		err = temporaryFile.Chmod(0o644)
	}
	if closeErr := temporaryFile.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temporaryFile.Name(), autostart.GetPath())
	}
	if err != nil {
		os.Remove(temporaryFile.Name())
	}
	return err
}

func (autostart *XDGAutostart) Unregister() error {
	if err := os.Remove(autostart.GetPath()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (autostart *XDGAutostart) CanOpenSystemSettings() bool { return false }

func (autostart *XDGAutostart) OpenSystemSettings() error { return ErrUnavailable }

// quoteExecArgument follows the Desktop Entry spec: reserved characters need double quotes, with \ " ` $ escaped inside.
func quoteExecArgument(argument string) string {
	if argument != "" && !strings.ContainsAny(argument, " \t\n\"'\\><~|&;$*?#()`") {
		return argument
	}
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`)
	return `"` + replacer.Replace(argument) + `"`
}
