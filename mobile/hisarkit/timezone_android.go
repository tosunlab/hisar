package hisarkit

// Go on Android reads zones only from /system and /data; newer Android keeps its time zone data in a module instead, so
// SetTimeZone falls back to the copy built into Hisar.
import _ "time/tzdata"
