package browser

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tryhisar/hisar/core/browserprotocol"
)

type passkeyRecordingHandler struct {
	passkeyRequest browserprotocol.PasskeyRequest
}

func (handler *passkeyRecordingHandler) HandleRequest(_ context.Context, request *browserprotocol.Request) (any, error) {
	if err := json.Unmarshal(request.Passkey, &handler.passkeyRequest); err != nil {
		return nil, browserprotocol.CreateError(browserprotocol.ErrorInvalidRequest, err.Error())
	}
	return browserprotocol.PasskeyListResult{AllPasskey: []browserprotocol.PasskeySummary{{CredentialID: "AQ", Title: "Example"}}}, nil
}

func TestPasskeyParametersPassThrough(t *testing.T) {
	handler := &passkeyRecordingHandler{}
	server := CreateServer(handler, nil, "")
	response := server.answerMessage(context.Background(), []byte(`{"id":"1","type":"passkeys.list","token":"t","passkey":{"origin":"https://example.com","relyingPartyId":"example.com","allowCredentials":["AQ","Ag"],"isCrossOrigin":true,"topOrigin":"https://top.example"}}`))
	if !response.OK {
		t.Fatalf("response: %+v", response)
	}
	received := handler.passkeyRequest
	if received.Origin != "https://example.com" || received.RelyingPartyID != "example.com" || len(received.AllAllowedCredentialID) != 2 || !received.IsCrossOrigin || received.TopOrigin != "https://top.example" {
		t.Errorf("passkey parameters: %+v", received)
	}
	encoded, _ := json.Marshal(response)
	if string(encoded) != `{"id":"1","ok":true,"result":{"passkeys":[{"credentialId":"AQ","vaultID":"","itemID":"","title":"Example","userName":"","userDisplayName":""}]}}` {
		t.Errorf("encoded response: %s", encoded)
	}
}
