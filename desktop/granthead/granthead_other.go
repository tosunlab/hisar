//go:build !darwin || !cgo

package granthead

import "github.com/tryhisar/hisar/core/app"

// New returns no store: the dialog then offers no grant that outlives a restart.
func New(service string) app.AgentGrantHeadStore {
	return nil
}
