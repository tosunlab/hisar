//go:build !darwin || !cgo || !sparkle

package updater

const isSparkleLinked = false

func startSparkle() {}

func setSparkleSession(feedURL, authorization string) {}

func setSparkleAutomaticCheck(isEnabled bool) {}

func checkSparkleForUpdates() bool { return false }
