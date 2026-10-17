package cliprotocol

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// testdata/launcher.json is the launcher's wire contract; the Raycast extension's tests read the same file.
type launcherGolden struct {
	RequestTypes []string `json:"requestTypes"`
	Fields       []string `json:"fields"`
	ErrorCodes   []string `json:"errorCodes"`
	Exchanges    []struct {
		Request    json.RawMessage `json:"request"`
		ResultType string          `json:"resultType"`
		Result     json.RawMessage `json:"result"`
	} `json:"exchanges"`
	Errors []json.RawMessage `json:"errors"`
}

func readLauncherGolden(t *testing.T) launcherGolden {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "launcher.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden launcherGolden
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	return golden
}

// requireSameWire decodes data into target refusing unknown fields, encodes it again and expects the same JSON.
func requireSameWire(t *testing.T, name string, data json.RawMessage, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	encoded, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if !bytes.Equal(wantJSON, gotJSON) {
		t.Errorf("%s: the Go types encode\n%s\nbut the contract says\n%s", name, gotJSON, wantJSON)
	}
}

func TestLauncherWireMatchesTheContractTheExtensionReads(t *testing.T) {
	golden := readLauncherGolden(t)
	if !slices.Equal(golden.RequestTypes, ListLauncherRequestTypes()) {
		t.Errorf("contract request types %v, app %v", golden.RequestTypes, ListLauncherRequestTypes())
	}
	for _, field := range golden.Fields {
		if !IsLauncherField(field) {
			t.Errorf("the app refuses to copy %q", field)
		}
	}
	allErrorCode := []string{ErrorInvalidRequest, ErrorIntegrationDisabled, ErrorSignedOut, ErrorLocked, ErrorDenied, ErrorOffline, ErrorNotFound, ErrorInternal, ErrorUpdateRequired}
	if !slices.Equal(golden.ErrorCodes, allErrorCode) {
		t.Errorf("contract error codes %v, app %v", golden.ErrorCodes, allErrorCode)
	}
	resultByType := map[string]func() any{
		"HelloResult":             func() any { return &HelloResult{} },
		"LauncherHelloResult":     func() any { return &LauncherHelloResult{} },
		"LauncherItemListResult":  func() any { return &LauncherItemListResult{} },
		"CodeResult":              func() any { return &CodeResult{} },
		"LauncherPasswordResult":  func() any { return &LauncherPasswordResult{} },
		"LauncherCopyResult":      func() any { return &LauncherCopyResult{} },
		"GeneratedPasswordResult": func() any { return &GeneratedPasswordResult{} },
		"Empty":                   func() any { return &struct{}{} },
	}
	for _, exchange := range golden.Exchanges {
		var request Request
		requireSameWire(t, "request", exchange.Request, &request)
		if request.Type != TypeHello && !slices.Contains(ListLauncherRequestTypes(), request.Type) {
			t.Errorf("%q is not a launcher request", request.Type)
		}
		newResult, isKnown := resultByType[exchange.ResultType]
		if !isKnown {
			t.Fatalf("unknown result type %q", exchange.ResultType)
		}
		requireSameWire(t, exchange.ResultType, exchange.Result, newResult())
	}
	for _, response := range golden.Errors {
		var decoded Response
		requireSameWire(t, "error response", response, &decoded)
		if decoded.Error == nil || !slices.Contains(allErrorCode, decoded.Error.Code) {
			t.Errorf("error response %s has no known code", response)
		}
	}
}
