package main

import "os"

// The console sends Ctrl-C to the command itself; hisar only must not exit before it.
var allForwardedSignal = []os.Signal{os.Interrupt}

func isTerminalSignal(os.Signal) bool { return true }

func checkForegroundJob() bool { return true }
