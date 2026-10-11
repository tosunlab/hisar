package vault

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
