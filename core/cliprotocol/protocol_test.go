package cliprotocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"path/filepath"
	"testing"
)

func TestMessageRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	request := Request{ID: "1", Type: TypeGetItem, VaultID: "vault", ItemID: "item"}
	if err := WriteJSON(&buffer, request); err != nil {
		t.Fatal(err)
	}
	if length := binary.BigEndian.Uint32(buffer.Bytes()[:4]); int(length) != buffer.Len()-4 {
		t.Fatalf("length prefix %d for %d bytes", length, buffer.Len()-4)
	}
	var decoded Request
	if err := ReadJSON(&buffer, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != "1" || decoded.Type != TypeGetItem || decoded.VaultID != "vault" || decoded.ItemID != "item" {
		t.Errorf("decoded %+v", decoded)
	}
}

func TestReadMessageRefusesOversizedFrame(t *testing.T) {
	var buffer bytes.Buffer
	binary.Write(&buffer, binary.BigEndian, uint32(MaxMessageSize+1))
	if _, err := ReadMessage(&buffer); !errors.Is(err, ErrMessageTooLarge) {
		t.Errorf("oversized frame: %v", err)
	}
	if err := WriteMessage(&buffer, make([]byte, MaxMessageSize+1)); !errors.Is(err, ErrMessageTooLarge) {
		t.Errorf("oversized write: %v", err)
	}
}

func TestReadJSONReportsInvalidJSON(t *testing.T) {
	var buffer bytes.Buffer
	WriteMessage(&buffer, []byte("{not json"))
	var decoded Request
	if err := ReadJSON(&buffer, &decoded); err == nil {
		t.Error("invalid JSON accepted")
	}
}

func TestGetSocketPath(t *testing.T) {
	t.Setenv(SocketEnvironment, "/tmp/other.sock")
	if path, err := GetSocketPath(); err != nil || path != "/tmp/other.sock" {
		t.Errorf("override: %q %v", path, err)
	}
	t.Setenv(SocketEnvironment, "")
	t.Setenv("HOME", "/home/someone")
	if path, err := GetSocketPath(); err != nil || path != filepath.Join("/home/someone", ".hisar", "cli.sock") {
		t.Errorf("default: %q %v", path, err)
	}
}

func TestResponseErrorMessage(t *testing.T) {
	var err error = CreateError(ErrorDenied, "denied in Hisar")
	var responseError *ResponseError
	if !errors.As(err, &responseError) || responseError.Code != ErrorDenied || err.Error() != "denied in Hisar" {
		t.Errorf("error: %v", err)
	}
}

func TestResolveSecretRequestEncoding(t *testing.T) {
	var buffer bytes.Buffer
	request := Request{ID: "2", Type: TypeResolveSecret, References: []string{"hisar://Personal/Example/password"}, CommandLine: []string{"npm", "test"}}
	if err := WriteJSON(&buffer, request); err != nil {
		t.Fatal(err)
	}
	encoded := string(buffer.Bytes()[4:])
	if encoded != `{"id":"2","type":"secrets.resolve","references":["hisar://Personal/Example/password"],"commandLine":["npm","test"]}` {
		t.Errorf("encoded %s", encoded)
	}
}
