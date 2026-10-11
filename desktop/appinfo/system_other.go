//go:build !(darwin && !ios && cgo)

package appinfo

import "github.com/tryhisar/hisar/core/sshagent"

func CreateSystemResolver() *Resolver {
	system := System{
		ProcessReader: ProcessReader{
			GetPath:             sshagent.GetProcessPath,
			GetParentPID:        sshagent.GetParentPID,
			ListAllArgument:     ListAllProcessArgument,
			GetWorkingDirectory: GetProcessWorkingDirectory,
		},
	}
	setPlatformProcessFunction(&system)
	return CreateResolver(system)
}
