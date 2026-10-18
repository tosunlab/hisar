package localsocket

import (
	"errors"
	"net"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

type SystemPeerResolver struct{}

func (SystemPeerResolver) ResolvePeer(connection net.Conn) (PeerInfo, error) {
	pid, err := GetPipeClientPID(connection)
	if err != nil {
		return PeerInfo{}, err
	}
	path, err := GetProcessPath(pid)
	if err != nil {
		return PeerInfo{PID: pid, Name: UnknownPeerName}, nil
	}
	return PeerInfo{PID: pid, Path: path, Name: filepath.Base(path)}, nil
}

func GetProcessPath(pid int) (string, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(process)
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buffer[:size]), nil
}

// GetParentPID returns the parent of a process, 0 when it has none, from a Toolhelp32 snapshot.
func GetParentPID(pid int) (int, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if int(entry.ProcessID) == pid {
			return int(entry.ParentProcessID), nil
		}
	}
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return 0, errors.New("localsocket: no such process")
	}
	return 0, err
}
