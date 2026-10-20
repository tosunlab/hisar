package agentharness

import "errors"

// maximumChainLength ends a walk that never reaches launchd or init; such a chain is unknown.
const maximumChainLength = 64

var errNoProcess = errors.New("agentharness: no such process")

// Instance is what identifies a process instance: its start time, its parent, and whether it already exited (a zombie
// waiting for its parent).
type Instance struct {
	StartTime int64
	ParentPID int
	IsExited  bool
}

// Reader reads the processes of a chain. ReadInstance must succeed for every process; the other reads may fail, and nil
// functions leave their fields empty. A process whose path and arguments cannot be read belongs to another user, so it
// cannot be the user's harness.
type Reader struct {
	ReadInstance        func(pid int) (Instance, error)
	GetPath             func(pid int) (string, error)
	ListArguments       func(pid int) ([]string, error)
	GetWorkingDirectory func(pid int) (string, error)
	GetSignature        func(path string) Signature
}

// ReadChain walks from pid to launchd or init, then reads every process again: a process that is gone, exited, has
// another start time or another parent makes the chain unknown, and so does a parent that started after its child,
// which is a PID reused during the walk.
func ReadChain(pid int, reader Reader) Chain {
	processes := []Process{}
	for pid > 1 {
		if len(processes) == maximumChainLength {
			return Chain{Processes: processes}
		}
		instance, err := reader.ReadInstance(pid)
		if err != nil || instance.IsExited {
			return Chain{Processes: processes}
		}
		if count := len(processes); count > 0 && instance.StartTime > processes[count-1].StartTime {
			return Chain{Processes: processes}
		}
		processes = append(processes, reader.readProcess(pid, instance))
		pid = instance.ParentPID
	}
	for _, process := range processes {
		instance, err := reader.ReadInstance(process.PID)
		if err != nil || instance != (Instance{StartTime: process.StartTime, ParentPID: process.ParentPID}) {
			return Chain{Processes: processes}
		}
	}
	return Chain{Processes: processes, IsKnown: true}
}

func (reader Reader) readProcess(pid int, instance Instance) Process {
	process := Process{PID: pid, ParentPID: instance.ParentPID, StartTime: instance.StartTime}
	if path, err := reader.GetPath(pid); err == nil {
		process.Path = path
	}
	if reader.ListArguments != nil {
		if arguments, err := reader.ListArguments(pid); err == nil {
			process.Arguments = arguments
		}
	}
	if reader.GetWorkingDirectory != nil {
		if workingDirectory, err := reader.GetWorkingDirectory(pid); err == nil {
			process.WorkingDirectory = workingDirectory
		}
	}
	if reader.GetSignature != nil && process.Path != "" {
		process.Signature = reader.GetSignature(process.Path)
	}
	return process
}
