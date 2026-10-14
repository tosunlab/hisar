package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
)

// fakeAPIServer stores encrypted vaults and items in memory, like the Hisar server API.
type fakeAPIServer struct {
	mutex                      sync.Mutex
	allVault                   []apitypes.Vault
	allItemByVaultID           map[string]map[string]apitypes.Item
	seq                        int64
	accountID                  string
	role                       string
	allMember                  []apitypes.Member
	allAccessByVaultID         map[string][]apitypes.RotatedAccess
	allDevice                  []apitypes.Device
	isUnreachable              bool
	eventChannel               chan string
	allShareInvitation         []apitypes.SentShareInvitation
	allReceivedShareInvitation []apitypes.ReceivedShareInvitation
	allWaitingShareInvitation  []apitypes.WaitingShareInvitation
	allAnsweredShareInvitation []string
	allInvitedEmail            []string
	isEmailUnavailable         bool
	approvalEventChannel       chan apiclient.Event
	eventStreamCount           int
	allApprovalByRequestID     map[string]*fakeApproval
	failingApprovalPostCount   int
	deviceListHangingChannel   chan struct{}
}

func (server *fakeAPIServer) setUnreachable(isUnreachable bool) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.isUnreachable = isUnreachable
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	json.NewEncoder(writer).Encode(value)
}

func (server *fakeAPIServer) createHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/account", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		writeJSON(writer, http.StatusOK, apitypes.Account{AccountID: server.accountID, Role: server.role})
	})
	mux.HandleFunc("GET /api/v1/vaults", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		writeJSON(writer, http.StatusOK, server.allVault)
	})
	mux.HandleFunc("POST /api/v1/vaults", func(writer http.ResponseWriter, request *http.Request) {
		var createRequest apitypes.CreateVaultRequest
		if err := json.NewDecoder(request.Body).Decode(&createRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		server.allVault = append(server.allVault, apitypes.Vault{ID: createRequest.ID, Type: createRequest.Type, EncName: createRequest.EncName, Role: apitypes.VaultOwner, WrappedKey: createRequest.WrappedKey})
		server.allItemByVaultID[createRequest.ID] = map[string]apitypes.Item{}
		server.allAccessByVaultID[createRequest.ID] = []apitypes.RotatedAccess{{AccountID: server.accountID, Role: apitypes.VaultOwner, WrappedKey: createRequest.WrappedKey}}
		writer.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("GET /api/v1/vaults/{vaultID}/changes", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		allItem := []apitypes.Item{}
		for _, storedItem := range server.allItemByVaultID[request.PathValue("vaultID")] {
			allItem = append(allItem, storedItem)
		}
		writeJSON(writer, http.StatusOK, apitypes.ChangesResponse{Items: allItem, NextSeq: server.seq})
	})
	mux.HandleFunc("PUT /api/v1/vaults/{vaultID}/items/{itemID}", func(writer http.ResponseWriter, request *http.Request) {
		var putRequest apitypes.PutItemRequest
		if err := json.NewDecoder(request.Body).Decode(&putRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		allItem, found := server.allItemByVaultID[request.PathValue("vaultID")]
		if !found {
			writeJSON(writer, http.StatusNotFound, apitypes.Error{Error: "vault not found"})
			return
		}
		itemID := request.PathValue("itemID")
		current, exists := allItem[itemID]
		baseRevision := strings.Trim(request.Header.Get("If-Match"), `"`)
		if (exists && current.Revision != baseRevision) || (!exists && baseRevision != "") {
			conflict := apitypes.ConflictResponse{}
			if exists {
				conflict.Current = &current
			}
			writeJSON(writer, http.StatusConflict, conflict)
			return
		}
		server.seq++
		allItem[itemID] = apitypes.Item{ID: itemID, Revision: putRequest.Revision, Seq: server.seq, Deleted: putRequest.Deleted, Overview: putRequest.Overview, Details: putRequest.Details}
		writeJSON(writer, http.StatusOK, apitypes.PutItemResponse{Seq: server.seq})
	})
	mux.HandleFunc("GET /api/v1/events", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		eventChannel := server.eventChannel
		server.mutex.Unlock()
		if eventChannel == nil {
			http.NotFound(writer, request)
			return
		}
		server.mutex.Lock()
		approvalEventChannel := server.approvalEventChannel
		server.eventStreamCount++
		server.mutex.Unlock()
		writer.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(writer, ": connected\n\n")
		writer.(http.Flusher).Flush()
		for {
			select {
			case <-request.Context().Done():
				return
			case eventName := <-eventChannel:
				fmt.Fprintf(writer, "event: %s\ndata: {}\n\n", eventName)
				writer.(http.Flusher).Flush()
			case approvalEvent := <-approvalEventChannel:
				fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", approvalEvent.Name, approvalEvent.Data)
				writer.(http.Flusher).Flush()
			}
		}
	})
	server.addAllSharingRoute(mux)
	server.addAllApprovalRoute(mux)
	server.addAllVaultManagementRoute(mux)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		isUnreachable := server.isUnreachable
		server.mutex.Unlock()
		if isUnreachable {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		mux.ServeHTTP(writer, request)
	})
}
