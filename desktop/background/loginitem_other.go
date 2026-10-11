//go:build !linux && !(darwin && !ios && cgo)

package background

func CreateLoginItem(homeDirectory string) LoginItem { return unavailableLoginItem{} }
