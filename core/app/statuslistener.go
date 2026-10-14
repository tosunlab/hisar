package app

// AddStatusListener calls listener on its own goroutine after Hisar unlocks or locks; not a method, so the UI cannot call it.
func AddStatusListener(service *Service, listener func()) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.statusListeners = append(service.statusListeners, listener)
}

// notifyStatusListener runs with the service mutex held, so listeners that read the status must not run inline.
func (s *Service) notifyStatusListener() {
	for _, listener := range s.statusListeners {
		go listener()
	}
}
