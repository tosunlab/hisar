package app

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

func IsExactHostMatch(s *Service) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.state.IsExactHostMatch
}

func SetExactHostMatch(s *Service, isExact bool) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.state.IsExactHostMatch = isExact
	return s.state.Save()
}

// ReloadExactHostMatch reads the setting another app-group process may have saved.
func ReloadExactHostMatch(s *Service) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	state, err := localstate.Load(s.config.StatePath)
	if err != nil {
		return err
	}
	s.state.IsExactHostMatch = state.IsExactHostMatch
	return nil
}
