// Package emergencykit renders the printable Hisar Emergency Kit PDF.
package emergencykit

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tryhisar/hisar/core/crypt"

	"github.com/signintech/gopdf"
	"github.com/skip2/go-qrcode"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

type Kit struct {
	Name      string
	Email     string
	Server    string
	SecretKey string
	CreatedAt time.Time
}

const (
	pageWidth   = 595.28
	pageHeight  = 841.89
	pageMargin  = 48.0
	contentWide = pageWidth - 2*pageMargin
)

var ErrIncompleteKit = errors.New("emergencykit: server, email and Secret Key are required")

type signInPayload struct {
	Server    string `json:"server"`
	Email     string `json:"email"`
	SecretKey string `json:"secretKey"`
}

// BuildQRPayload encodes sign-in details as plain text, never a dispatchable URI.
func BuildQRPayload(kit Kit) string {
	data, _ := json.Marshal(signInPayload{Server: kit.Server, Email: kit.Email, SecretKey: kit.SecretKey})
	return "HISARKIT1 " + base64.RawURLEncoding.EncodeToString(data)
}

func Generate(kit Kit) ([]byte, error) {
	if strings.TrimSpace(kit.Server) == "" || strings.TrimSpace(kit.Email) == "" || strings.TrimSpace(kit.SecretKey) == "" {
		return nil, ErrIncompleteKit
	}
	server, err := url.Parse(kit.Server)
	if err != nil || server.Hostname() == "" || (server.Scheme != "https" && server.Scheme != "http") ||
		server.User != nil || server.Fragment != "" || !utf8.ValidString(kit.Email) {
		return nil, errors.New("emergencykit: invalid sign-in details")
	}
	if _, err := crypt.ParseSecretKey(kit.SecretKey); err != nil {
		return nil, fmt.Errorf("emergencykit: %w", err)
	}
	if kit.CreatedAt.IsZero() {
		kit.CreatedAt = time.Now()
	}
	document := &gopdf.GoPdf{}
	document.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	document.SetInfo(gopdf.PdfInfo{Title: "Hisar Emergency Kit", Author: "Hisar", Creator: "Hisar", CreationDate: kit.CreatedAt})
	for fontName, fontData := range map[string][]byte{"regular": goregular.TTF, "bold": gobold.TTF, "mono": gomono.TTF} {
		if err := document.AddTTFFontData(fontName, fontData); err != nil {
			return nil, err
		}
	}
	document.AddPage()

	writer := &pageWriter{document: document}
	writer.drawHeader(kit)
	writer.drawIntroduction()
	if err := writer.drawSignInSection(kit); err != nil {
		return nil, err
	}
	writer.drawSecretKey(kit.SecretKey)
	writer.drawMasterPasswordField()
	writer.drawSignInSteps()
	writer.drawFooter()
	if writer.err != nil {
		return nil, writer.err
	}
	return document.GetBytesPdf(), nil
}

// pageWriter keeps the first drawing error so the layout code stays linear.
type pageWriter struct {
	document *gopdf.GoPdf
	cursorY  float64
	err      error
}

func (writer *pageWriter) setFont(fontName string, size float64) {
	if err := writer.document.SetFont(fontName, "", size); err != nil && writer.err == nil {
		writer.err = err
	}
}

func (writer *pageWriter) writeText(x, y float64, text string) {
	writer.document.SetXY(x, y)
	if err := writer.document.Cell(nil, text); err != nil && writer.err == nil {
		writer.err = err
	}
}

// wrapText breaks at spaces; a single word wider than the line is broken by characters.
func (writer *pageWriter) wrapText(text string, width float64) []string {
	measure := func(candidate string) float64 {
		measured, err := writer.document.MeasureTextWidth(candidate)
		if err != nil && writer.err == nil {
			writer.err = err
		}
		return measured
	}
	var lines []string
	currentLine := ""
	for _, word := range strings.Fields(text) {
		candidate := word
		if currentLine != "" {
			candidate = currentLine + " " + word
		}
		if measure(candidate) <= width {
			currentLine = candidate
			continue
		}
		if currentLine != "" {
			lines = append(lines, currentLine)
		}
		currentLine = word
		for measure(currentLine) > width && len([]rune(currentLine)) > 1 {
			runes := []rune(currentLine)
			splitIndex := len(runes) - 1
			for splitIndex > 1 && measure(string(runes[:splitIndex])) > width {
				splitIndex--
			}
			lines = append(lines, string(runes[:splitIndex]))
			currentLine = string(runes[splitIndex:])
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	return lines
}

func (writer *pageWriter) writeParagraph(x, y, width float64, text string, lineHeight float64) float64 {
	for _, line := range writer.wrapText(text, width) {
		writer.writeText(x, y, line)
		y += lineHeight
	}
	return y
}

func (writer *pageWriter) fillRectangle(x, y, width, height, radius float64) {
	if err := writer.document.Rectangle(x, y, x+width, y+height, "F", radius, 8); err != nil && writer.err == nil {
		writer.err = err
	}
}

func (writer *pageWriter) strokeRectangle(x, y, width, height, radius float64) {
	if err := writer.document.Rectangle(x, y, x+width, y+height, "D", radius, 8); err != nil && writer.err == nil {
		writer.err = err
	}
}

// drawLogo draws the mark in its print variant: dark square with a light tower, `size` points wide.
func (writer *pageWriter) drawLogo(x, y, size float64) {
	scale := size / 32
	writer.document.SetFillColor(11, 12, 14)
	writer.fillRectangle(x, y, size, size, 8*scale)
	point := func(pathX, pathY float64) gopdf.Point { return gopdf.Point{X: x + pathX*scale, Y: y + pathY*scale} }
	writer.document.SetFillColor(242, 243, 245)
	writer.document.Polygon([]gopdf.Point{
		point(8, 26), point(8, 12), point(11, 12), point(11, 9), point(13.5, 9), point(13.5, 12),
		point(18.5, 12), point(18.5, 9), point(21, 9), point(21, 12), point(24, 12), point(24, 26),
	}, "F")
	writer.document.SetFillColor(11, 12, 14)
	writer.fillRectangle(x+14*scale, y+21*scale, 4*scale, 5*scale, 0)
	writer.document.Oval(x+14*scale, y+19*scale, x+18*scale, y+23*scale)
}

func (writer *pageWriter) drawHeader(kit Kit) {
	writer.drawLogo(pageMargin, pageMargin, 40)
	writer.document.SetTextColor(11, 12, 14)
	writer.setFont("bold", 22)
	writer.writeText(pageMargin+54, pageMargin+2, "Hisar Emergency Kit")
	writer.document.SetTextColor(100, 106, 115)
	writer.setFont("regular", 10.5)
	createdFor := "Created on " + kit.CreatedAt.Format("2 January 2006")
	if strings.TrimSpace(kit.Name) != "" {
		createdFor = fmt.Sprintf("Created for %s on %s", strings.TrimSpace(kit.Name), kit.CreatedAt.Format("2 January 2006"))
	}
	writer.writeText(pageMargin+54, pageMargin+28, createdFor)
	writer.cursorY = pageMargin + 64
}

func (writer *pageWriter) drawIntroduction() {
	writer.document.SetTextColor(40, 44, 50)
	writer.setFont("regular", 11)
	writer.cursorY = writer.writeParagraph(pageMargin, writer.cursorY, contentWide,
		"Use this kit to sign in to Hisar on a new device. Print it and keep it somewhere safe, for example with your important documents. "+
			"Anyone who has this kit and your master password can open your vaults.", 16)
	writer.cursorY += 14
}

func (writer *pageWriter) drawLabel(x, y float64, label string) {
	writer.document.SetTextColor(100, 106, 115)
	writer.setFont("bold", 8.5)
	writer.writeText(x, y, strings.ToUpper(label))
}

func (writer *pageWriter) drawSignInSection(kit Kit) error {
	const qrSize = 132.0
	sectionTop := writer.cursorY
	sectionHeight := qrSize + 28
	writer.document.SetStrokeColor(214, 217, 222)
	writer.document.SetLineWidth(1)
	writer.strokeRectangle(pageMargin, sectionTop, contentWide, sectionHeight, 10)

	textX := pageMargin + 20
	textWidth := contentWide - qrSize - 60
	writer.drawLabel(textX, sectionTop+22, "Sign-in address")
	writer.document.SetTextColor(11, 12, 14)
	writer.setFont("regular", 13)
	nextY := writer.writeParagraph(textX, sectionTop+38, textWidth, kit.Server, 17)
	writer.drawLabel(textX, nextY+12, "Email")
	writer.document.SetTextColor(11, 12, 14)
	writer.setFont("regular", 13)
	writer.writeParagraph(textX, nextY+28, textWidth, kit.Email, 17)

	qrCode, err := qrcode.New(BuildQRPayload(kit), qrcode.Medium)
	if err != nil {
		return err
	}
	qrCode.DisableBorder = true
	moduleRows := qrCode.Bitmap()
	moduleSize := qrSize / float64(len(moduleRows))
	qrX := pageMargin + contentWide - qrSize - 14
	qrY := sectionTop + 14
	writer.document.SetFillColor(11, 12, 14)
	// One rectangle per run of dark modules, slightly overlapping vertically, avoids hairline seams.
	for rowIndex, modules := range moduleRows {
		for runStart := 0; runStart < len(modules); runStart++ {
			if !modules[runStart] {
				continue
			}
			runEnd := runStart
			for runEnd+1 < len(modules) && modules[runEnd+1] {
				runEnd++
			}
			writer.fillRectangle(qrX+float64(runStart)*moduleSize, qrY+float64(rowIndex)*moduleSize, float64(runEnd-runStart+1)*moduleSize, moduleSize+0.4, 0)
			runStart = runEnd
		}
	}
	writer.document.SetTextColor(100, 106, 115)
	writer.setFont("regular", 8.5)
	writer.writeParagraph(textX, sectionTop+sectionHeight-45, textWidth,
		"Scan only with Hisar. This code contains your Secret Key, not your master password. Do not search or share its text.", 11)
	writer.cursorY = sectionTop + sectionHeight + 22
	return nil
}

func (writer *pageWriter) drawSecretKey(secretKey string) {
	writer.drawLabel(pageMargin, writer.cursorY, "Secret Key")
	boxTop := writer.cursorY + 16
	writer.document.SetFillColor(240, 241, 243)
	writer.fillRectangle(pageMargin, boxTop, contentWide, 54, 10)
	writer.document.SetTextColor(11, 12, 14)
	writer.setFont("mono", 17)
	keyWidth, err := writer.document.MeasureTextWidth(secretKey)
	if err != nil && writer.err == nil {
		writer.err = err
	}
	writer.writeText(pageMargin+(contentWide-keyWidth)/2, boxTop+17, secretKey)
	writer.document.SetTextColor(100, 106, 115)
	writer.setFont("regular", 9.5)
	writer.cursorY = writer.writeParagraph(pageMargin, boxTop+66, contentWide,
		"Your Secret Key never leaves your devices. Nobody, including Hisar, can recover it for you.", 13)
	writer.cursorY += 18
}

func (writer *pageWriter) drawMasterPasswordField() {
	writer.drawLabel(pageMargin, writer.cursorY, "Master password")
	lineY := writer.cursorY + 52
	writer.document.SetStrokeColor(150, 155, 163)
	writer.document.SetLineWidth(1)
	writer.document.Line(pageMargin, lineY, pageMargin+contentWide, lineY)
	writer.document.SetTextColor(100, 106, 115)
	writer.setFont("regular", 9.5)
	writer.writeText(pageMargin, lineY+8, "Write it by hand. Never store your master password in a file on a computer.")
	writer.cursorY = lineY + 40
}

func (writer *pageWriter) drawSignInSteps() {
	writer.cursorY += 8
	writer.document.SetTextColor(11, 12, 14)
	writer.setFont("bold", 13)
	writer.writeText(pageMargin, writer.cursorY, "Signing in on a new device")
	writer.cursorY += 28
	steps := []string{
		"Install Hisar and open it.",
		"Choose Sign in, then enter the sign-in address, email and Secret Key from this kit.",
		"Enter your master password. Hisar stores the Secret Key on that device, so you only need this kit again for the next new device.",
	}
	for stepIndex, step := range steps {
		writer.document.SetFillColor(240, 241, 243)
		writer.fillRectangle(pageMargin, writer.cursorY-3, 20, 20, 10)
		writer.document.SetTextColor(11, 12, 14)
		writer.setFont("bold", 10)
		writer.writeText(pageMargin+7, writer.cursorY+1, fmt.Sprint(stepIndex+1))
		writer.setFont("regular", 11)
		writer.cursorY = writer.writeParagraph(pageMargin+32, writer.cursorY, contentWide-32, step, 15) + 12
	}
}

func (writer *pageWriter) drawFooter() {
	writer.document.SetStrokeColor(214, 217, 222)
	writer.document.SetLineWidth(0.8)
	footerY := pageHeight - pageMargin - 30
	writer.document.Line(pageMargin, footerY, pageMargin+contentWide, footerY)
	writer.document.SetTextColor(100, 106, 115)
	writer.setFont("regular", 9)
	writer.writeText(pageMargin, footerY+10, "Hisar · end-to-end encrypted password manager for developers")
	writer.writeText(pageMargin, footerY+23, "A forgotten master password cannot be recovered by anyone, including Hisar. Keep this kit somewhere safe.")
}
