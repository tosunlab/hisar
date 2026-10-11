//go:build !linux && !(darwin && cgo)

package background

func CreateLoginItem(homeDirectory string) LoginItem { return unavailableLoginItem{} }
