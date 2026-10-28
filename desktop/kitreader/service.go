// Package kitreader reads sign-in details from a locally chosen Emergency Kit PDF.
package kitreader

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/tryhisar/hisar/core/crypt"
)

const maxPDFBytes = 16 << 20

// AccountFields contains only the account fields a kit can fill, never a master password.
type AccountFields struct {
	Server    string `json:"server"`
	Email     string `json:"email"`
	SecretKey string `json:"secretKey"`
}

type Service struct {
	choosePDF func() (string, error)
}

func New(choosePDF func() (string, error)) *Service {
	return &Service{choosePDF: choosePDF}
}

// Open returns nil when the user cancels. It never contacts the server named in a kit.
func (service *Service) Open() (*AccountFields, error) {
	path, err := service.choosePDF()
	if err != nil {
		return nil, errors.New("The Emergency Kit could not be opened. Try choosing the PDF again.")
	}
	if path == "" {
		return nil, nil
	}
	return readPDF(path)
}

func readPDF(path string) (*AccountFields, error) {
	file, err := openPDF(path)
	if err != nil {
		return nil, errors.New("The Emergency Kit could not be read. Check that you can open the PDF.")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("Choose an Emergency Kit PDF file.")
	}
	if info.Size() > maxPDFBytes {
		return nil, errors.New("The PDF is too large. Choose an Emergency Kit PDF smaller than 16 MB.")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxPDFBytes+1))
	if err != nil || len(data) > maxPDFBytes {
		return nil, errors.New("The Emergency Kit could not be read. Choose a PDF smaller than 16 MB.")
	}
	codes := decodePDF(data)
	for _, code := range codes {
		if fields := parseCode(code); fields != nil {
			return fields, nil
		}
	}
	return nil, errors.New("This PDF does not contain a valid Hisar Emergency Kit QR code. Choose your Emergency Kit PDF or enter its details manually.")
}

func parseCode(text string) *AccountFields {
	if len(text) > 8192 || !utf8.ValidString(text) {
		return nil
	}
	if strings.HasPrefix(text, "HISARKIT") {
		if !strings.HasPrefix(text, "HISARKIT1 ") {
			return nil
		}
		encoded := strings.TrimPrefix(text, "HISARKIT1 ")
		data, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
		if err != nil || len(data) == 0 || base64.RawURLEncoding.EncodeToString(data) != encoded || !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
			return nil
		}
		var fields map[string]string
		if json.Unmarshal(data, &fields) != nil || len(fields) != 3 {
			return nil
		}
		account := &AccountFields{Server: fields["server"], Email: fields["email"], SecretKey: fields["secretKey"]}
		server, err := url.Parse(account.Server)
		if err != nil || server.Hostname() == "" || (strings.ToLower(server.Scheme) != "https" && strings.ToLower(server.Scheme) != "http") || server.User != nil || strings.Contains(account.Server, "#") || strings.TrimSpace(account.Email) == "" {
			return nil
		}
		if _, err := crypt.ParseSecretKey(account.SecretKey); err != nil {
			return nil
		}
		return account
	}
	link, err := url.Parse(strings.TrimSpace(text))
	if err != nil || !strings.EqualFold(link.Scheme, "hisar") || !strings.EqualFold(link.Host, "sign-in") {
		return nil
	}
	// URL query items in printed legacy kits treat a literal plus as a plus, not a form-encoded space.
	values, err := url.ParseQuery(strings.ReplaceAll(link.RawQuery, "+", "%2B"))
	if err != nil {
		return nil
	}
	account := &AccountFields{Server: values.Get("server"), Email: values.Get("email"), SecretKey: values.Get("secretKey")}
	if account.Server == "" && account.Email == "" && account.SecretKey == "" {
		return nil
	}
	return account
}
