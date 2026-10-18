package cliprotocol

import (
	"os"

	"github.com/tryhisar/hisar/core/localsocket"
)

func GetDefaultSocketPath(homeDirectory string) string {
	return localsocket.GetDefaultPath(homeDirectory, "cli")
}

// GetSocketPath returns HISAR_CLI_SOCKET when set, otherwise ~/.hisar/cli.sock (a named pipe on Windows).
func GetSocketPath() (string, error) {
	if path := os.Getenv(SocketEnvironment); path != "" {
		return path, nil
	}
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return GetDefaultSocketPath(homeDirectory), nil
}
