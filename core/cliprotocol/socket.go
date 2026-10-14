package cliprotocol

import (
	"os"
	"path/filepath"
)

func GetDefaultSocketPath(homeDirectory string) string {
	return filepath.Join(homeDirectory, ".hisar", "cli.sock")
}

// GetSocketPath returns HISAR_CLI_SOCKET when set, otherwise ~/.hisar/cli.sock.
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
