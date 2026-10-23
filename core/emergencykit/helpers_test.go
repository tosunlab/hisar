package emergencykit

import (
	"compress/zlib"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// printedLine is one line of text on the generated page, in PDF points with the origin at the bottom left.
type printedLine struct {
	text        string
	left, right float64
	bottom, top float64
}

func (line printedLine) overlaps(other printedLine) bool {
	return line.left < other.right && other.left < line.right && line.bottom < other.top && other.bottom < line.top
}

type pdfFont struct {
	widths          map[uint16]float64
	runes           map[uint16]string
	ascent, descent float64
}

var (
	pdfObjectPattern    = regexp.MustCompile(`(?s)(\d+) 0 obj(.*?)endobj`)
	pdfStreamPattern    = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	pdfFontRefPattern   = regexp.MustCompile(`/F(\d+) (\d+) 0 R`)
	pdfTextPattern      = regexp.MustCompile(`BT\s+(-?[\d.]+) (-?[\d.]+) TD\s+/F(\d+) ([\d.]+) Tf[^\[]*\[<([0-9A-Fa-f]*)>\] TJ\s+ET`)
	pdfWidthsPattern    = regexp.MustCompile(`(\d+)\s*\[([\d.\s]+)\]`)
	pdfCharacterPattern = regexp.MustCompile(`<([0-9A-Fa-f]{4})>\s*<([0-9A-Fa-f]{4})>\s*<([0-9A-Fa-f]{4})>`)
)

func readPDFReference(object, key string) string {
	if match := regexp.MustCompile(`/` + key + `\s*\[?\s*(\d+) 0 R`).FindStringSubmatch(object); match != nil {
		return match[1]
	}
	return ""
}

func readPDFNumber(t *testing.T, object, key string) float64 {
	t.Helper()
	match := regexp.MustCompile(`/` + key + `\s+(-?[\d.]+)`).FindStringSubmatch(object)
	if match == nil {
		t.Fatalf("PDF object has no /%s: %s", key, object)
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func readPDFStream(t *testing.T, object string) string {
	t.Helper()
	match := pdfStreamPattern.FindStringSubmatch(object)
	if match == nil {
		return ""
	}
	if !strings.Contains(object, "/FlateDecode") {
		return match[1]
	}
	reader, err := zlib.NewReader(strings.NewReader(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// readPrintedLines reads every text line Generate drew, with its extent from the embedded fonts' metrics.
func readPrintedLines(t *testing.T, document []byte) []printedLine {
	t.Helper()
	objects := map[string]string{}
	for _, match := range pdfObjectPattern.FindAllSubmatch(document, -1) {
		objects[string(match[1])] = string(match[2])
	}
	fonts := map[string]pdfFont{}
	var lines []printedLine
	for _, object := range objects {
		if strings.Contains(object, "/Font <<") {
			for _, reference := range pdfFontRefPattern.FindAllStringSubmatch(object, -1) {
				fonts[reference[1]] = readPDFFont(t, objects, objects[reference[2]])
			}
		}
	}
	for _, object := range objects {
		content := readPDFStream(t, object)
		for _, match := range pdfTextPattern.FindAllStringSubmatch(content, -1) {
			font, ok := fonts[match[3]]
			if !ok {
				t.Fatalf("text uses an unknown font F%s", match[3])
			}
			x, _ := strconv.ParseFloat(match[1], 64)
			baseline, _ := strconv.ParseFloat(match[2], 64)
			size, _ := strconv.ParseFloat(match[4], 64)
			var text strings.Builder
			width := 0.0
			for index := 0; index+4 <= len(match[5]); index += 4 {
				glyph, _ := strconv.ParseUint(match[5][index:index+4], 16, 16)
				text.WriteString(font.runes[uint16(glyph)])
				width += font.widths[uint16(glyph)] / 1000 * size
			}
			lines = append(lines, printedLine{
				text: text.String(), left: x, right: x + width,
				bottom: baseline + font.descent/1000*size, top: baseline + font.ascent/1000*size,
			})
		}
	}
	if len(lines) == 0 {
		t.Fatal("no text found in the PDF")
	}
	return lines
}

func readPDFFont(t *testing.T, objects map[string]string, object string) pdfFont {
	t.Helper()
	descendant := objects[readPDFReference(object, "DescendantFonts")]
	descriptor := objects[readPDFReference(descendant, "FontDescriptor")]
	font := pdfFont{
		widths:  map[uint16]float64{},
		runes:   map[uint16]string{},
		ascent:  readPDFNumber(t, descriptor, "Ascent"),
		descent: readPDFNumber(t, descriptor, "Descent"),
	}
	widths := regexp.MustCompile(`(?s)/W\s*\[(.*)\]`).FindStringSubmatch(descendant)
	if widths == nil {
		t.Fatalf("font has no widths: %s", descendant)
	}
	for _, run := range pdfWidthsPattern.FindAllStringSubmatch(widths[1], -1) {
		first, _ := strconv.Atoi(run[1])
		for offset, field := range strings.Fields(run[2]) {
			width, _ := strconv.ParseFloat(field, 64)
			font.widths[uint16(first+offset)] = width
		}
	}
	toUnicode := readPDFStream(t, objects[readPDFReference(object, "ToUnicode")])
	for _, entry := range pdfCharacterPattern.FindAllStringSubmatch(toUnicode, -1) {
		first, _ := strconv.ParseUint(entry[1], 16, 16)
		last, _ := strconv.ParseUint(entry[2], 16, 16)
		character, _ := strconv.ParseUint(entry[3], 16, 32)
		for glyph := first; glyph <= last; glyph++ {
			font.runes[uint16(glyph)] = string(rune(character + glyph - first))
		}
	}
	return font
}
