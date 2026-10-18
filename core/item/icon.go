package item

import (
	"bytes"
	"image/png"
	"slices"
)

const (
	// MaxIconImageSize and MaxIconImageSide bound an icon image, which lives in the item's overview.
	MaxIconImageSize = 24 << 10
	MaxIconImageSide = 128
)

// Icon is the icon the user chose for an item: a preset glyph in one of the icon colors, or a PNG image, which wins.
// A Preset this app does not know (from a newer app) is kept and drawn as the category icon.
type Icon struct {
	Preset string `json:"preset,omitempty"`
	Color  string `json:"color,omitempty"`
	Image  []byte `json:"image,omitempty"`
}

// IconPresets returns the IDs of the preset icons, in the order the pickers show them.
func IconPresets() []string {
	return []string{
		"bank", "card", "wallet", "crypto", "cart", "gift", "mail", "chat", "social", "work", "home", "school",
		"cloud", "server", "database", "code", "terminal", "wifi", "globe", "game", "music", "video", "health",
		"travel", "car", "key", "lock", "shield", "star", "heart",
	}
}

// IconColors returns the IDs of the colors a preset icon can have; "" is the neutral tile.
func IconColors() []string {
	return []string{"red", "orange", "yellow", "green", "teal", "blue", "indigo", "purple", "pink", "gray"}
}

// IsIconImage reports whether data is a PNG that fits the icon limits.
func IsIconImage(data []byte) bool {
	if len(data) == 0 || len(data) > MaxIconImageSize {
		return false
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	return err == nil && config.Width > 0 && config.Height > 0 && config.Width <= MaxIconImageSide && config.Height <= MaxIconImageSide
}

// NormalizeIcon returns the icon to keep: nil for an empty one, the image alone when it has one, or the preset with a
// known color (an unknown color becomes the neutral tile). An image outside the limits is dropped.
func NormalizeIcon(icon *Icon) *Icon {
	if icon == nil {
		return nil
	}
	if IsIconImage(icon.Image) {
		return &Icon{Image: bytes.Clone(icon.Image)}
	}
	if !isIconPresetID(icon.Preset) {
		return nil
	}
	color := icon.Color
	if !slices.Contains(IconColors(), color) {
		color = ""
	}
	return &Icon{Preset: icon.Preset, Color: color}
}

// isIconPresetID reports whether preset looks like a preset ID: up to 40 lower-case letters, digits and dashes.
func isIconPresetID(preset string) bool {
	if preset == "" || len(preset) > 40 {
		return false
	}
	for _, character := range preset {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}
