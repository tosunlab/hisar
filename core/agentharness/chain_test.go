package agentharness

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"sync"
	"testing"
	"time"
)

type fakeProcess struct {
	instance  Instance
	path      string
	arguments []string
}

// fakeSystem is a process table; changes run once each, after the read of their PID, to change the table mid-walk.
type fakeSystem struct {
	mutex     sync.Mutex
	processes map[int]fakeProcess
	changes   map[int]func(processes map[int]fakeProcess)
}

func (system *fakeSystem) createReader() Reader {
	return Reader{
		ReadInstance: func(pid int) (Instance, error) {
			system.mutex.Lock()
			defer system.mutex.Unlock()
			process, isFound := system.processes[pid]
			if change, isChanged := system.changes[pid]; isChanged {
				delete(system.changes, pid)
				change(system.processes)
			}
			if !isFound {
				return Instance{}, errNoProcess
			}
			return process.instance, nil
		},
		GetPath: func(pid int) (string, error) {
			system.mutex.Lock()
			defer system.mutex.Unlock()
			return system.processes[pid].path, nil
		},
		ListArguments: func(pid int) ([]string, error) {
			system.mutex.Lock()
			defer system.mutex.Unlock()
			return system.processes[pid].arguments, nil
		},
		GetSignature: func(path string) Signature {
			if path == ompPath {
				return ompSignature
			}
			return Signature{}
		},
	}
}

func createOmpInTernSystem() *fakeSystem {
	return &fakeSystem{
		processes: map[int]fakeProcess{
			39200: {instance: Instance{StartTime: 300, ParentPID: 39136}, path: "/opt/homebrew/bin/hisar", arguments: []string{"hisar", "vaults"}},
			39136: {instance: Instance{StartTime: 200, ParentPID: 1226}, path: "/bin/sh", arguments: []string{"sh", "-c", "hisar vaults"}},
			1226:  {instance: Instance{StartTime: 100, ParentPID: 1217}, path: ompPath, arguments: []string{"omp"}},
			1217:  {instance: Instance{StartTime: 50, ParentPID: 1}, path: ternPath, arguments: []string{"tern", "daemon"}},
		},
		changes: map[int]func(map[int]fakeProcess){},
	}
}

func TestReadChainIsASnapshotOfInstances(t *testing.T) {
	chain := ReadChain(39200, createOmpInTernSystem().createReader())
	pids := []int{}
	for _, process := range chain.Processes {
		pids = append(pids, process.PID)
	}
	if !chain.IsKnown || !slices.Equal(pids, []int{39200, 39136, 1226, 1217}) {
		t.Fatalf("chain = %+v", chain)
	}
	if omp := chain.Processes[2]; omp.StartTime != 100 || omp.ParentPID != 1217 || omp.Path != ompPath || omp.Signature != ompSignature || !slices.Equal(omp.Arguments, []string{"omp"}) {
		t.Errorf("omp = %+v", omp)
	}
	if session, isFound, _ := Find(chain); !isFound || session.PID != 1226 || session.StartTime != 100 {
		t.Errorf("session = %+v, %v", session, isFound)
	}
}

func TestChangeDuringTheWalkIsUnknown(t *testing.T) {
	cases := map[string]func(system *fakeSystem){
		"omp exits and sh is adopted by launchd": func(system *fakeSystem) {
			system.changes[1217] = func(processes map[int]fakeProcess) {
				delete(processes, 1226)
				shell := processes[39136]
				shell.instance.ParentPID = 1
				processes[39136] = shell
			}
		},
		"omp's PID is reused by a newer process": func(system *fakeSystem) {
			system.changes[39136] = func(processes map[int]fakeProcess) {
				processes[1226] = fakeProcess{instance: Instance{StartTime: 400, ParentPID: 1217}, path: "/usr/bin/true"}
			}
		},
		"omp exited and waits for Tern": func(system *fakeSystem) {
			omp := system.processes[1226]
			omp.instance.IsExited = true
			system.processes[1226] = omp
		},
		"a process cannot be read": func(system *fakeSystem) {
			delete(system.processes, 1217)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			system := createOmpInTernSystem()
			change(system)
			chain := ReadChain(39200, system.createReader())
			if chain.IsKnown {
				t.Errorf("known chain %+v", chain)
			}
			if session, isFound, _ := Find(chain); isFound {
				t.Errorf("session %+v", session)
			}
		})
	}
}

func TestEndlessChainIsUnknown(t *testing.T) {
	system := &fakeSystem{processes: map[int]fakeProcess{}, changes: map[int]func(map[int]fakeProcess){}}
	for pid := 2; pid < 2+maximumChainLength+1; pid++ {
		system.processes[pid] = fakeProcess{instance: Instance{StartTime: int64(1000 - pid), ParentPID: pid + 1}, path: "/bin/sh"}
	}
	if chain := ReadChain(2, system.createReader()); chain.IsKnown || len(chain.Processes) != maximumChainLength {
		t.Errorf("chain of %d processes, known %v", len(chain.Processes), chain.IsKnown)
	}
}

func TestReadThisProcess(t *testing.T) {
	chain := ReadChain(os.Getpid(), SystemReader())
	if !chain.IsKnown || len(chain.Processes) == 0 {
		t.Fatalf("chain = %+v", chain)
	}
	own := chain.Processes[0]
	if own.PID != os.Getpid() || own.ParentPID != os.Getppid() || own.StartTime == 0 || !slices.Equal(own.Arguments, os.Args) {
		t.Errorf("this process = %+v", own)
	}
	if len(chain.Processes) > 1 && chain.Processes[1].PID != os.Getppid() {
		t.Errorf("parent = %+v", chain.Processes[1])
	}
}

func startChild(t *testing.T) (*exec.Cmd, Session) {
	t.Helper()
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { child.Process.Kill(); child.Wait() })
	instance, err := ReadInstance(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	own, err := ReadInstance(os.Getpid())
	if err != nil || instance.StartTime < own.StartTime {
		t.Fatalf("child started at %d, before this process at %d: %v", instance.StartTime, own.StartTime, err)
	}
	return child, Session{PID: child.Process.Pid, StartTime: instance.StartTime}
}

func TestWaitForExit(t *testing.T) {
	child, session := startChild(t)
	if !IsRunning(session) {
		t.Fatal("the child is not running")
	}
	if IsRunning(Session{PID: session.PID, StartTime: session.StartTime + 1}) {
		t.Error("another instance of the PID runs")
	}
	exited := make(chan struct{})
	go func() {
		WaitForExit(context.Background(), session)
		close(exited)
	}()
	select {
	case <-exited:
		t.Fatal("returned before the exit")
	case <-time.After(100 * time.Millisecond):
	}
	child.Process.Kill()
	child.Wait()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("the exit was not seen")
	}
	if IsRunning(session) {
		t.Error("an exited child runs")
	}

	_, session = startChild(t)
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		WaitForExit(ctx, session)
		close(stopped)
	}()
	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForExit ignored its context")
	}
}

func TestParseKernelProcessArgument(t *testing.T) {
	raw := []byte{3, 0, 0, 0}
	raw = append(raw, "/usr/bin/git\x00\x00\x00\x00git\x00push\x00\x00HOME=/Users/developer\x00GITHUB_TOKEN=secret\x00"...)
	arguments, err := parseKernelProcessArgument(raw)
	if err != nil || !slices.Equal(arguments, []string{"git", "push", ""}) {
		t.Errorf("arguments %q, %v", arguments, err)
	}
	for _, malformed := range [][]byte{{1, 0}, {1, 0, 0, 0, 'x'}, {0xff, 0xff, 0, 0, '/', 0}} {
		if _, err := parseKernelProcessArgument(malformed); err == nil {
			t.Errorf("%q parsed", malformed)
		}
	}
	truncated := append([]byte{5, 0, 0, 0}, "/bin/ls\x00ls\x00-l"...)
	if arguments, err := parseKernelProcessArgument(truncated); err != nil || !slices.Equal(arguments, []string{"ls", "-l"}) {
		t.Errorf("truncated: %q, %v", arguments, err)
	}
}

func TestParseLinuxCommandLine(t *testing.T) {
	if arguments := parseLinuxCommandLine([]byte("git\x00push\x00origin\x00")); !slices.Equal(arguments, []string{"git", "push", "origin"}) {
		t.Errorf("%q", arguments)
	}
	if arguments := parseLinuxCommandLine(nil); len(arguments) != 0 {
		t.Errorf("empty: %q", arguments)
	}
}
