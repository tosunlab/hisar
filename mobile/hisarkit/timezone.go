package hisarkit

import "time"

// SetTimeZone takes the device's IANA zone name ("Europe/Istanbul") for the dates Hisar writes: Go on Android starts in
// UTC. Call it once, before New; a later zone change waits for the next launch, as time.Local must not change while
// other goroutines read it.
func SetTimeZone(name string) error {
	location, err := time.LoadLocation(name)
	if err != nil {
		return err
	}
	time.Local = location
	return nil
}
