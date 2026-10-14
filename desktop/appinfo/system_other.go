//go:build !(darwin && !ios && cgo)

package appinfo

import "github.com/tryhisar/hisar/core/localsocket"

func CreateSystemResolver() *Resolver {
	system := System{
		ProcessReader: ProcessReader{
			GetPath:             localsocket.GetProcessPath,
			GetParentPID:        localsocket.GetParentPID,
			ListAllArgument:     ListAllProcessArgument,
			GetWorkingDirectory: GetProcessWorkingDirectory,
		},
	}
	setPlatformProcessFunction(&system)
	return CreateResolver(system)
}
