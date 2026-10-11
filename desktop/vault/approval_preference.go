package vault

import (
	"errors"
	"slices"
)

// ApprovalPreference keeps the approval switches in desktop.json; it is not a bound service, the agent's status exposes it.
type ApprovalPreference struct {
	service *Service
}

func CreateApprovalPreference(service *Service) *ApprovalPreference {
	return &ApprovalPreference{service: service}
}

func (preference *ApprovalPreference) IsCompactSSHApproval() bool {
	s := preference.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.state.IsCompactSSHApproval
}

func (preference *ApprovalPreference) SetCompactSSHApproval(isCompact bool) error {
	s := preference.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.state.IsCompactSSHApproval = isCompact
	return s.state.Save()
}

func (preference *ApprovalPreference) IsPhoneApproval() bool {
	s := preference.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return !s.state.IsPhoneApprovalDisabled
}

func (preference *ApprovalPreference) SetPhoneApproval(isEnabled bool) error {
	s := preference.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.state.IsPhoneApprovalDisabled = !isEnabled
	return s.state.Save()
}

var allSigningRememberSeconds = []int{0, 600, 3600}

// GetSigningRememberSeconds is how long an approved commit signature may be remembered; 0 always asks.
func (preference *ApprovalPreference) GetSigningRememberSeconds() int {
	s := preference.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !slices.Contains(allSigningRememberSeconds, s.state.SigningRememberSeconds) {
		return 0
	}
	return s.state.SigningRememberSeconds
}

func (preference *ApprovalPreference) SetSigningRememberSeconds(seconds int) error {
	if !slices.Contains(allSigningRememberSeconds, seconds) {
		return errors.New("choose Always ask, 10 minutes or 1 hour")
	}
	s := preference.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.state.SigningRememberSeconds = seconds
	return s.state.Save()
}
