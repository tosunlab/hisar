package agent

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/localsocket"
)

type fakeApprovalPreference struct {
	isCompact              bool
	isPhoneApprovalOff     bool
	signingRememberSeconds int
	saveError              error
}

func (preference *fakeApprovalPreference) IsCompactSSHApproval() bool { return preference.isCompact }

func (preference *fakeApprovalPreference) SetCompactSSHApproval(isCompact bool) error {
	if preference.saveError != nil {
		return preference.saveError
	}
	preference.isCompact = isCompact
	return nil
}

func (preference *fakeApprovalPreference) IsPhoneApproval() bool {
	return !preference.isPhoneApprovalOff
}

func (preference *fakeApprovalPreference) SetPhoneApproval(isEnabled bool) error {
	if preference.saveError != nil {
		return preference.saveError
	}
	preference.isPhoneApprovalOff = !isEnabled
	return nil
}

func (preference *fakeApprovalPreference) GetSigningRememberSeconds() int {
	return preference.signingRememberSeconds
}

func (preference *fakeApprovalPreference) SetSigningRememberSeconds(seconds int) error {
	if preference.saveError != nil {
		return preference.saveError
	}
	preference.signingRememberSeconds = seconds
	return nil
}

func TestServiceSigningRememberClearsRememberedSigning(t *testing.T) {
	preference := &fakeApprovalPreference{}
	test := createSigningTest(t, 600, nil)
	service := CreateService(Config{HomeDirectory: t.TempDir(), ApprovalPreference: preference, Approver: test.approver})
	if service.GetStatus().SigningRememberSeconds != 0 {
		t.Error("commit signing is remembered by default")
	}
	test.approveInDialog(t, createSigningRequest(), true)
	if status, err := service.SetSigningRemember(3600); err != nil || status.SigningRememberSeconds != 3600 {
		t.Errorf("set: %+v %v", status, err)
	}
	if test.isSignedWithoutAsking(t, createSigningRequest()) {
		t.Error("a signature remembered before the setting changed is still used")
	}
}

func TestServicePhoneApprovalSwitch(t *testing.T) {
	preference := &fakeApprovalPreference{}
	service := CreateService(Config{HomeDirectory: t.TempDir(), ApprovalPreference: preference})
	if !service.GetStatus().IsPhoneApproval {
		t.Error("phone approvals are off by default")
	}
	if status, err := service.SetPhoneApproval(false); err != nil || status.IsPhoneApproval {
		t.Errorf("turn off: %+v %v", status, err)
	}
	preference.saveError = errors.New("disk full")
	if status, err := service.SetPhoneApproval(true); err == nil || status.IsPhoneApproval {
		t.Errorf("failed save: %+v %v", status, err)
	}
	withoutPreference := CreateService(Config{HomeDirectory: t.TempDir()})
	if status, err := withoutPreference.SetPhoneApproval(true); err == nil || status.IsPhoneApproval {
		t.Errorf("the switch changed without a preference store: %+v", status)
	}
}

func TestServiceCompactApprovalSwitchAndWindowFit(t *testing.T) {
	preference := &fakeApprovalPreference{}
	allFit := []string{}
	service := CreateService(Config{HomeDirectory: t.TempDir(), ApprovalPreference: preference, FitApprovalWindow: func(isCompact bool, height int) {
		allFit = append(allFit, fmt.Sprint(isCompact, height))
	}})
	if service.GetStatus().IsCompactApproval {
		t.Error("compact approvals on by default")
	}
	if status, err := service.SetCompactSSHApproval(true); err != nil || !status.IsCompactApproval {
		t.Errorf("turn on: %+v %v", status, err)
	}
	preference.saveError = errors.New("disk full")
	if status, err := service.SetCompactSSHApproval(false); err == nil || !status.IsCompactApproval {
		t.Errorf("failed save: %+v %v", status, err)
	}
	service.FitSSHApprovalWindow(true, 112)
	service.FitSSHApprovalWindow(false, 0)
	service.FitSSHApprovalWindow(false, 404)
	if strings.Join(allFit, ",") != "true 112,false 404" {
		t.Errorf("fits = %v", allFit)
	}
	withoutPreference := CreateService(Config{HomeDirectory: t.TempDir()})
	if _, err := withoutPreference.SetCompactSSHApproval(true); err == nil {
		t.Error("the switch changed without a preference store")
	}
	withoutPreference.FitSSHApprovalWindow(true, 100)
}

type fixedPeerResolver struct {
	peer localsocket.PeerInfo
	err  error
}

func (resolver fixedPeerResolver) ResolvePeer(net.Conn) (localsocket.PeerInfo, error) {
	return resolver.peer, resolver.err
}

func TestApplicationPeerResolver(t *testing.T) {
	findApplicationPath := func(pid int) (string, bool) {
		if pid == 42 {
			return "/System/Applications/Utilities/Terminal.app", true
		}
		return "", false
	}
	resolver := ApplicationPeerResolver{Base: fixedPeerResolver{peer: localsocket.PeerInfo{PID: 42, Path: "/usr/bin/ssh", Name: "ssh"}}, FindApplicationPath: findApplicationPath}
	if peer, err := resolver.ResolvePeer(nil); err != nil || peer.ApplicationPath != "/System/Applications/Utilities/Terminal.app" || peer.Path != "/usr/bin/ssh" {
		t.Errorf("peer = %+v %v", peer, err)
	}
	resolver.Base = fixedPeerResolver{peer: localsocket.PeerInfo{PID: 43, Path: "/usr/bin/ssh"}}
	if peer, _ := resolver.ResolvePeer(nil); peer.ApplicationPath != "" {
		t.Errorf("a peer without an app got %q", peer.ApplicationPath)
	}
	resolver.Base = fixedPeerResolver{err: errors.New("no descriptor")}
	if _, err := resolver.ResolvePeer(nil); err == nil {
		t.Error("the base error was dropped")
	}
}
