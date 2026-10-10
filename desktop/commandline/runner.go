package commandline

import (
	"context"
	"sync"
)

// Runner starts and stops the server as the integration switch changes.
type Runner struct {
	server     *Server
	socketPath string
	mutex      sync.Mutex
	cancel     context.CancelFunc
	stopped    chan struct{}
	serveError error
}

type RunnerStatus struct {
	SocketPath   string
	IsRunning    bool
	ErrorMessage string
}

func CreateRunner(server *Server, socketPath string) *Runner {
	return &Runner{server: server, socketPath: socketPath}
}

func (runner *Runner) Start() {
	runner.mutex.Lock()
	defer runner.mutex.Unlock()
	if runner.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	runner.cancel, runner.stopped, runner.serveError = cancel, stopped, nil
	go func() {
		defer close(stopped)
		err := runner.server.Serve(ctx, runner.socketPath)
		runner.mutex.Lock()
		defer runner.mutex.Unlock()
		if runner.stopped == stopped {
			runner.serveError = err
			if err != nil {
				runner.cancel, runner.stopped = nil, nil
			}
		}
	}()
}

// Stop closes the socket and every open connection, and ends every approval.
func (runner *Runner) Stop() {
	runner.mutex.Lock()
	cancel, stopped := runner.cancel, runner.stopped
	runner.cancel, runner.stopped = nil, nil
	runner.mutex.Unlock()
	if cancel != nil {
		cancel()
		<-stopped
	}
	runner.server.ClearAllApproval()
}

func (runner *Runner) GetStatus() RunnerStatus {
	runner.mutex.Lock()
	defer runner.mutex.Unlock()
	status := RunnerStatus{SocketPath: runner.socketPath, IsRunning: runner.cancel != nil}
	if runner.serveError != nil {
		status.ErrorMessage = runner.serveError.Error()
	}
	return status
}
