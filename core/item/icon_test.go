package item

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"reflect"
	"testing"
)

func encodeTestPNG(t *testing.T, side int) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, side, side))
	for x := range side {
		for y := range side {
			picture.Set(x, y, color.RGBA{uint8(x), uint8(y), 200, 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestNormalizeIcon(t *testing.T) {
	small := encodeTestPNG(t, 64)
	var jpegImage bytes.Buffer
	if err := jpeg.Encode(&jpegImage, image.NewRGBA(image.Rect(0, 0, 32, 32)), nil); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		icon *Icon
		want *Icon
	}{
		{"nil", nil, nil},
		{"empty", &Icon{}, nil},
		{"preset", &Icon{Preset: "bank", Color: "green"}, &Icon{Preset: "bank", Color: "green"}},
		{"unknown color", &Icon{Preset: "bank", Color: "chartreuse"}, &Icon{Preset: "bank"}},
		{"preset from a newer app", &Icon{Preset: "rocket"}, &Icon{Preset: "rocket"}},
		{"preset that is no ID", &Icon{Preset: "Bank <script>"}, nil},
		{"image wins", &Icon{Preset: "bank", Color: "green", Image: small}, &Icon{Image: small}},
		{"too large an image", &Icon{Image: encodeTestPNG(t, 200)}, nil},
		{"not a PNG", &Icon{Image: jpegImage.Bytes()}, nil},
		{"bad image falls back to its preset", &Icon{Preset: "bank", Image: []byte("not an image")}, &Icon{Preset: "bank"}},
	} {
		if got := NormalizeIcon(test.icon); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s: NormalizeIcon = %+v, want %+v", test.name, got, test.want)
		}
	}
}

func TestIconPresetsAreIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, preset := range IconPresets() {
		if !isIconPresetID(preset) || seen[preset] {
			t.Errorf("preset %q", preset)
		}
		seen[preset] = true
	}
}
