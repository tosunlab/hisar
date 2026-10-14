//go:build !(darwin && !ios && cgo)

package appinfo

import "github.com/tryhisar/hisar/core/localsocket"

func NewSystemResolver() *Resolver {
	system := System{
		ProcessReader: ProcessReader{
			GetPath:             localsocket.GetProcessPath,
			GetParentPID:        localsocket.GetParentPID,
			ListArguments:       ListProcessArguments,
			GetWorkingDirectory: GetProcessWorkingDirectory,
		},
	}
	setPlatformProcessFunction(&system)
	return NewResolver(system)
}
