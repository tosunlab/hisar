package commandline

import (
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

// IdentifyApp is the app the CLI socket approves a caller without a terminal as: the app its process chain resolves
// to, but only when macOS verified that app's code signature, including its sealed resources, as Apple's or a developer's.
// An unsigned app, a signature that does not verify, a background process or a process without an app has no identity,
// so its approval covers only its own connection, and it cannot be a launcher. Only the bundle on disk is checked.
func IdentifyApp(requester appinfo.Requester, resolver *appinfo.Resolver) (clisocket.AppIdentity, bool) {
	if requester.Kind != appinfo.RequesterKindApp || requester.App.Path == "" {
		return clisocket.AppIdentity{}, false
	}
	signature := resolver.GetStrictSignature(requester.App.Path)
	switch signature.Status {
	case appinfo.SignatureStatusApple, appinfo.SignatureStatusDeveloper:
		return clisocket.AppIdentity{Path: requester.App.Path, TeamID: signature.TeamID}, true
	}
	return clisocket.AppIdentity{}, false
}
