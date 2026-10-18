package main

// helperPaths are the binaries the app ships beside itself.
type helperPaths struct {
	browserHost string
	commandLine string
	// isCommandLineFromPackage means the package put hisar on the PATH, so Settings has nothing to install.
	isCommandLineFromPackage bool
}
