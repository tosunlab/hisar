package main

// helperPaths are the binaries and the omp plugin the app ships beside itself; ompPlugin is empty without one.
type helperPaths struct {
	browserHost string
	commandLine string
	ompPlugin   string
	// isCommandLineFromPackage means the package put hisar on the PATH, so Settings has nothing to install.
	isCommandLineFromPackage bool
}
