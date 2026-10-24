//go:build !(darwin && !ios && cgo)

package axfill

import (
	"github.com/tryhisar/hisar/core/signin"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

// New returns nil: agents sign in to websites only in Hisar for Mac.
func New(*appinfo.Resolver) signin.Driver {
	return nil
}
