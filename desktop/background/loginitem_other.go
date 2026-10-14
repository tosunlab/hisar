//go:build !linux && !(darwin && !ios && cgo)

package background

func NewLoginItem(homeDirectory string) LoginItem { return unavailableLoginItem{} }
