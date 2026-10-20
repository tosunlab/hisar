package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const programTimeout = 30 * time.Second

// A harness may start children of its own, as omp's plugin manager runs Bun; past this delay a child holding the
// output open is abandoned.
const programWaitDelay = 2 * time.Second

// findProgram looks on the login shell's PATH, then in the agent's own install locations.
func findProgram(name string, userPath, locations []string) (string, bool) {
	var candidates []string
	for _, directory := range userPath {
		if filepath.IsAbs(directory) {
			candidates = append(candidates, filepath.Join(directory, name))
		}
	}
	for _, candidate := range append(candidates, locations...) {
		if isExecutable(candidate) {
			return candidate, true
		}
	}
	return "", false
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// program is an agent's command-line tool, found on this Mac.
type program struct {
	name      string
	path      string
	userPath  []string
	directory string
	timeout   time.Duration
}

// run starts the program in its agent's directory with the folder it is in first on the PATH, so a program installed
// by Bun or npm finds its runtime there.
func (program program) run(arguments ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), program.timeout)
	defer cancel()
	process := exec.CommandContext(ctx, program.path, arguments...)
	process.Dir = program.directory
	searchPath := append([]string{filepath.Dir(program.path)}, program.userPath...)
	process.Env = append(os.Environ(), "PATH="+strings.Join(searchPath, string(os.PathListSeparator)), "NO_COLOR=1")
	process.WaitDelay = programWaitDelay
	var stderr bytes.Buffer
	process.Stderr = &stderr
	output, err := process.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s did not answer within %s", program.name, program.timeout)
	}
	if err != nil {
		return nil, errors.New(describeFailure(stderr.String(), err))
	}
	return output, nil
}

// describeFailure keeps the program's last line, without omp's cross mark and the "Error:" of the error inside it.
func describeFailure(stderr string, err error) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	detail := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[len(lines)-1]), "✘"))
	detail = strings.ReplaceAll(detail, "Error: ", "")
	if detail == "" {
		return err.Error()
	}
	return detail
}
