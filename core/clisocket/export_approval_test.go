package clisocket

import (
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

func TestExportAlwaysAsks(t *testing.T) {
	fixture := createServerFixture(t)
	client := fixture.connect(t)
	if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}); !response.OK {
		t.Fatalf("vaults.list: %+v", response.Error)
	}
	exportRequest := cliprotocol.Request{Type: cliprotocol.TypeExport, Export: &cliprotocol.ExportChange{Format: cliprotocol.ExportFormatPlain, Path: "/tmp/hisar-export.json"}}
	for attempt := 1; attempt <= 2; attempt++ {
		if response := client.send(t, exportRequest); !response.OK {
			t.Fatalf("export: %+v", response.Error)
		}
		if count := fixture.approver.requestCount.Load(); count != int32(1+attempt) {
			t.Fatalf("after %d exports in an approved session: %d approvals", attempt, count)
		}
	}
	request := fixture.approver.allRequest[1]
	if request.Export == nil || !request.Export.IsPlain || request.Export.Path != "/tmp/hisar-export.json" || request.ShareLink != nil {
		t.Errorf("approval request %+v %+v", request, request.Export)
	}
	if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeExport}); getErrorCode(response) != cliprotocol.ErrorInvalidRequest {
		t.Errorf("export without details: %+v", response)
	}

	fixture.approver.result = ErrDenied
	before := len(fixture.handler.allType)
	if response := client.send(t, exportRequest); getErrorCode(response) != cliprotocol.ErrorDenied {
		t.Fatalf("denied export: %+v", response)
	}
	if slices.Contains(fixture.handler.allType[before:], cliprotocol.TypeExport) {
		t.Error("a denied export reached the handler")
	}
}
