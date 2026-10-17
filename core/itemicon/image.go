// Package itemicon turns uploaded images and website icons into the small PNGs items show, and fetches website icons
// from the sites themselves.
package itemicon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"  // registers the GIF decoder
	_ "image/jpeg" // registers the JPEG decoder
	"image/png"

	"golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers the WebP decoder

	"github.com/tryhisar/hisar/core/item"
)

const (
	// MaxSourceSize and MaxSourceSide bound the images NormalizeImage reads.
	MaxSourceSize = 5 << 20
	MaxSourceSide = 4096
)

// ErrUnsupportedImage is returned for data that is not a PNG, JPEG, GIF, WebP or ICO image within the limits.
var ErrUnsupportedImage = errors.New("choose a PNG, JPEG, GIF or WebP image")

// NormalizeImage crops an image to a centered square and scales it down to at most item.MaxIconImageSide, as a PNG of at
// most item.MaxIconImageSize. It never scales an image up.
func NormalizeImage(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > MaxSourceSize {
		return nil, ErrUnsupportedImage
	}
	source, err := decodeImage(data)
	if err != nil {
		return nil, ErrUnsupportedImage
	}
	bounds := source.Bounds()
	side := min(bounds.Dx(), bounds.Dy())
	if side <= 0 {
		return nil, ErrUnsupportedImage
	}
	square := image.Rect(0, 0, side, side).Add(image.Pt(bounds.Min.X+(bounds.Dx()-side)/2, bounds.Min.Y+(bounds.Dy()-side)/2))
	for _, target := range []int{item.MaxIconImageSide, 96, 64, 48, 32} {
		target = min(target, side)
		scaled := image.NewNRGBA(image.Rect(0, 0, target, target))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), source, square, draw.Src, nil)
		var encoded bytes.Buffer
		if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&encoded, scaled); err != nil {
			return nil, fmt.Errorf("itemicon: encoding: %w", err)
		}
		if item.IsIconImage(encoded.Bytes()) {
			return encoded.Bytes(), nil
		}
	}
	return nil, ErrUnsupportedImage
}

func decodeImage(data []byte) (image.Image, error) {
	if isICO(data) {
		return decodeICO(data)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if config.Width > MaxSourceSide || config.Height > MaxSourceSide {
		return nil, ErrUnsupportedImage
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	return decoded, err
}

func isICO(data []byte) bool {
	return len(data) >= 6 && binary.LittleEndian.Uint16(data[0:]) == 0 && binary.LittleEndian.Uint16(data[2:]) == 1 &&
		binary.LittleEndian.Uint16(data[4:]) > 0
}

// decodeICO decodes the largest image of an ICO file: a PNG, or a BMP stored without its file header.
func decodeICO(data []byte) (image.Image, error) {
	count := int(binary.LittleEndian.Uint16(data[4:]))
	if len(data) < 6+16*count {
		return nil, ErrUnsupportedImage
	}
	var best []byte
	bestSide, bestBits := 0, 0
	for index := range count {
		entry := data[6+16*index : 6+16*(index+1)]
		side := int(entry[0])
		if side == 0 {
			side = 256
		}
		bits := int(binary.LittleEndian.Uint16(entry[6:]))
		size, offset := binary.LittleEndian.Uint32(entry[8:]), binary.LittleEndian.Uint32(entry[12:])
		if uint64(offset)+uint64(size) > uint64(len(data)) || size < 40 {
			continue
		}
		if side > bestSide || (side == bestSide && bits > bestBits) {
			best, bestSide, bestBits = data[offset:offset+size], side, bits
		}
	}
	if best == nil {
		return nil, ErrUnsupportedImage
	}
	if bytes.HasPrefix(best, []byte("\x89PNG")) {
		return decodeImage(best)
	}
	return decodeICOBitmap(best)
}

// decodeICOBitmap decodes an ICO entry's BMP: a BITMAPINFOHEADER whose height counts the image and its AND mask twice.
func decodeICOBitmap(dib []byte) (image.Image, error) {
	headerSize := binary.LittleEndian.Uint32(dib[0:])
	width := int(int32(binary.LittleEndian.Uint32(dib[4:])))
	height := int(int32(binary.LittleEndian.Uint32(dib[8:]))) / 2
	bits := binary.LittleEndian.Uint16(dib[14:])
	if headerSize < 40 || uint32(len(dib)) < headerSize || width <= 0 || height <= 0 || width > 256 || height > 256 {
		return nil, ErrUnsupportedImage
	}
	if bits == 32 {
		// Bottom-up BGRA rows.
		pixels := dib[headerSize:]
		if len(pixels) < width*height*4 {
			return nil, ErrUnsupportedImage
		}
		decoded := image.NewNRGBA(image.Rect(0, 0, width, height))
		for y := range height {
			row := pixels[(height-1-y)*width*4:]
			for x := range width {
				decoded.SetNRGBA(x, y, color.NRGBA{R: row[x*4+2], G: row[x*4+1], B: row[x*4], A: row[x*4+3]})
			}
		}
		return decoded, nil
	}
	// Other depths: give the bitmap its file header and real height, and let the BMP decoder read it (without the mask).
	fixed := bytes.Clone(dib)
	binary.LittleEndian.PutUint32(fixed[8:], uint32(height))
	paletteSize := uint32(0)
	if bits <= 8 {
		colors := binary.LittleEndian.Uint32(dib[32:])
		if colors == 0 {
			colors = 1 << bits
		}
		paletteSize = colors * 4
	}
	header := make([]byte, 14)
	copy(header, "BM")
	binary.LittleEndian.PutUint32(header[2:], uint32(14+len(fixed)))
	binary.LittleEndian.PutUint32(header[10:], 14+headerSize+paletteSize)
	return bmp.Decode(bytes.NewReader(append(header, fixed...)))
}
