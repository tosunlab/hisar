package vault

import "github.com/tryhisar/hisar/core/localstate"

// ReadSettings returns this device's preferences; not a method, so the UI cannot call it.
func ReadSettings(s *Service) localstate.Settings {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.state.Settings
}

// UpdateSettings changes this device's preferences and saves them; not a method, so the UI cannot call it.
func UpdateSettings(s *Service, change func(settings *localstate.Settings)) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	change(&s.state.Settings)
	return s.state.Save()
}
