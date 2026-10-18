package localsocket

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func getCurrentUserSID() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("localsocket: reading this user's SID: %w", err)
	}
	return user.User.Sid, nil
}

func getProcessUserSID(pid int) (*windows.SID, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(process)
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return nil, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, err
	}
	return user.User.Sid, nil
}

// checkSameUser refuses a process that runs as another user, or one whose user cannot be read.
func checkSameUser(pid int) error {
	currentSID, err := getCurrentUserSID()
	if err != nil {
		return err
	}
	processSID, err := getProcessUserSID(pid)
	if err != nil {
		return fmt.Errorf("localsocket: reading the user of process %d: %w", pid, err)
	}
	if !windows.EqualSid(currentSID, processSID) {
		return fmt.Errorf("localsocket: process %d runs as another user (%s)", pid, processSID)
	}
	return nil
}
