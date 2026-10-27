package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

// fakeAPIServer stores encrypted vaults and items in memory, like the Hisar server API.
type fakeAPIServer struct {
	mutex                    sync.Mutex
	vaults                   []apitypes.Vault
	itemsByVaultID           map[string]map[string]apitypes.Item
	seq                      int64
	accountID                string
	account                  *apitypes.Account
	role                     string
	members                  []apitypes.Member
	accessesByVaultID        map[string][]apitypes.RotatedAccess
	devices                  []apitypes.Device
	isUnreachable            bool
	eventChannel             chan string
	shareInvitations         []apitypes.SentShareInvitation
	receivedShareInvitations []apitypes.ReceivedShareInvitation
	waitingShareInvitations  []apitypes.WaitingShareInvitation
	answeredShareInvitations []string
	invitedEmails            []string
	isEmailUnavailable       bool
	isInviteAdminsOnly       bool
	approvalEventChannel     chan apiclient.Event
	eventStreamCount         int
	approvalsByRequestID     map[string]*fakeApproval
	failingApprovalPostCount int
	deviceListHangingChannel chan struct{}
	preferences              apitypes.AccountPreferences
	vaultKeyRequests         []apitypes.VaultKeyRequest
	waitingVaultKeys         []apitypes.WaitingVaultKey
	setAccessRequests        []apitypes.SetAccessRequest
	isSetAccessFailing       bool
	// keyChangeRefusalCount refuses that many more items as sealed with a replaced key, as when rotations keep coming.
	keyChangeRefusalCount int
	// keyGenerationsSent records the generation of every item write, in order.
	keyGenerationsSent []int64
	// isSuspended answers the account and the event stream as the server does for a suspended account's session.
	isSuspended bool
	// historyFullMessage, when set, refuses item writes with it as the server's 507 for a vault whose history is full.
	historyFullMessage string
	// pushRelayURL is the relay the fake server names in /push/config; isPushUnsupported answers it 404, as an older
	// server does, and acceptsPayloadKey says it takes an iPhone's payload key. pushRegistration is the last PUT
	// /devices/current/push, nil after a DELETE.
	pushRelayURL      string
	isPushUnsupported bool
	acceptsPayloadKey bool
	pushRegistration  *apitypes.PushRegistration
	// isSessionEnded answers every request 401, as the server does once it ended this device's session.
	isSessionEnded bool
}

// refusesSuspended answers 403 with the suspension sentence while the account is suspended.
func (server *fakeAPIServer) refusesSuspended(writer http.ResponseWriter) bool {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if server.isSuspended {
		writeJSON(writer, http.StatusForbidden, apitypes.Error{Error: apitypes.AccountSuspendedMessage})
	}
	return server.isSuspended
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

// refusesKeyGeneration plays the server refusing items sealed with a key generation other than the vault's current one:
// required for organization vaults, checked for any vault when set. It needs server.mutex.
func (server *fakeAPIServer) refusesKeyGeneration(writer http.ResponseWriter, vaultID string, keyGeneration int64) bool {
	index := slices.IndexFunc(server.vaults, func(vault apitypes.Vault) bool { return vault.ID == vaultID })
	isAccepted := index < 0 || keyGeneration == server.vaults[index].KeyGeneration || keyGeneration == 0 && server.vaults[index].OrganizationID == "" && server.vaults[index].KeyGeneration <= 1
	if isAccepted && server.keyChangeRefusalCount == 0 {
		return false
	}
	if isAccepted {
		server.keyChangeRefusalCount--
	}
	writeJSON(writer, http.StatusConflict, apitypes.KeyChangedResponse{Error: "the vault key changed since the items were encrypted", IsKeyChanged: true})
	return true
}

func (server *fakeAPIServer) createHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/account", func(writer http.ResponseWriter, request *http.Request) {
		if server.refusesSuspended(writer) {
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if server.account != nil {
			writeJSON(writer, http.StatusOK, server.account)
			return
		}
		writeJSON(writer, http.StatusOK, apitypes.Account{AccountID: server.accountID, Role: server.role})
	})
	mux.HandleFunc("GET /api/v1/account/preferences", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		writeJSON(writer, http.StatusOK, server.preferences)
	})
	mux.HandleFunc("PUT /api/v1/account/preferences", func(writer http.ResponseWriter, request *http.Request) {
		var putRequest apitypes.PutAccountPreferencesRequest
		json.NewDecoder(request.Body).Decode(&putRequest)
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if putRequest.BaseRevision != server.preferences.Revision {
			writeJSON(writer, http.StatusConflict, apitypes.Error{Error: "conflict"})
			return
		}
		server.preferences = apitypes.AccountPreferences{Revision: server.preferences.Revision + 1, Ciphertext: putRequest.Ciphertext}
		writeJSON(writer, http.StatusOK, apitypes.PutAccountPreferencesResponse{Revision: server.preferences.Revision})
	})
	mux.HandleFunc("GET /api/v1/vaults", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		writeJSON(writer, http.StatusOK, server.vaults)
	})
	createVault := func(writer http.ResponseWriter, request *http.Request) {
		var createRequest apitypes.CreateVaultRequest
		if err := json.NewDecoder(request.Body).Decode(&createRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		server.vaults = append(server.vaults, apitypes.Vault{
			ID: createRequest.ID, Type: createRequest.Type, EncName: createRequest.EncName, Role: apitypes.VaultOwner, WrappedKey: createRequest.WrappedKey,
			OrganizationID: createRequest.OrganizationID, KeyGeneration: 1, AccessRevision: 2,
		})
		server.itemsByVaultID[createRequest.ID] = map[string]apitypes.Item{}
		server.accessesByVaultID[createRequest.ID] = []apitypes.RotatedAccess{{AccountID: server.accountID, Role: apitypes.VaultOwner, WrappedKey: createRequest.WrappedKey}}
		writer.WriteHeader(http.StatusCreated)
	}
	mux.HandleFunc("POST /api/v1/vaults", createVault)
	mux.HandleFunc("POST /api/v1/vaults/personal", createVault)
	mux.HandleFunc("GET /api/v1/vaults/{vaultID}/changes", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		items := []apitypes.Item{}
		for _, storedItem := range server.itemsByVaultID[request.PathValue("vaultID")] {
			items = append(items, storedItem)
		}
		generation := int64(1)
		for _, vault := range server.vaults {
			if vault.ID == request.PathValue("vaultID") {
				generation = max(int64(1), vault.KeyGeneration)
			}
		}
		writeJSON(writer, http.StatusOK, apitypes.ChangesResponse{Items: items, NextSeq: server.seq, KeyGeneration: generation})
	})
	putItem := func(writer http.ResponseWriter, request *http.Request) {
		var putRequest apitypes.PutItemRequest
		if err := json.NewDecoder(request.Body).Decode(&putRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if strings.HasSuffix(request.URL.Path, "/import") {
			members := []crypt.ImportMember{}
			for _, member := range server.accessesByVaultID[request.PathValue("vaultID")] {
				members = append(members, crypt.ImportMember{AccountID: member.AccountID, Role: member.Role})
			}
			if putRequest.ImportScope == "" || putRequest.ImportScope != crypt.ImportScope(request.PathValue("vaultID"), members) {
				writeJSON(writer, http.StatusPreconditionFailed, apitypes.Error{Error: "destination membership changed; review the import destination again"})
				return
			}
		}
		items, found := server.itemsByVaultID[request.PathValue("vaultID")]
		if !found {
			writeJSON(writer, http.StatusNotFound, apitypes.Error{Error: "vault not found"})
			return
		}
		itemID := request.PathValue("itemID")
		current, exists := items[itemID]
		baseRevision := strings.Trim(request.Header.Get("If-Match"), `"`)
		server.keyGenerationsSent = append(server.keyGenerationsSent, putRequest.KeyGeneration)
		if server.refusesKeyGeneration(writer, request.PathValue("vaultID"), putRequest.KeyGeneration) {
			return
		}
		if server.historyFullMessage != "" {
			writeJSON(writer, http.StatusInsufficientStorage, apitypes.Error{Error: server.historyFullMessage})
			return
		}
		if (exists && current.Revision != baseRevision) || (!exists && baseRevision != "") {
			conflict := apitypes.ConflictResponse{}
			if exists {
				conflict.Current = &current
			}
			writeJSON(writer, http.StatusConflict, conflict)
			return
		}
		server.seq++
		items[itemID] = apitypes.Item{ID: itemID, Revision: putRequest.Revision, Seq: server.seq, Deleted: putRequest.Deleted, Overview: putRequest.Overview, Details: putRequest.Details}
		writeJSON(writer, http.StatusOK, apitypes.PutItemResponse{Seq: server.seq})
	}
	mux.HandleFunc("PUT /api/v1/vaults/{vaultID}/items/{itemID}", putItem)
	mux.HandleFunc("PUT /api/v1/vaults/{vaultID}/items/{itemID}/import", putItem)
	mux.HandleFunc("POST /api/v1/vaults/{vaultID}/items/{itemID}/move", func(writer http.ResponseWriter, request *http.Request) {
		var moveRequest apitypes.MoveItemRequest
		if err := json.NewDecoder(request.Body).Decode(&moveRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		sourceItems, isSourceFound := server.itemsByVaultID[request.PathValue("vaultID")]
		targetItems, isTargetFound := server.itemsByVaultID[moveRequest.TargetVaultID]
		if !isSourceFound || !isTargetFound {
			writeJSON(writer, http.StatusNotFound, apitypes.Error{Error: "vault not found"})
			return
		}
		itemID := request.PathValue("itemID")
		if server.refusesKeyGeneration(writer, request.PathValue("vaultID"), moveRequest.Tombstone.KeyGeneration) ||
			server.refusesKeyGeneration(writer, moveRequest.TargetVaultID, moveRequest.Moved.KeyGeneration) {
			return
		}
		current, exists := sourceItems[itemID]
		if !exists || current.Deleted || current.Revision != moveRequest.BaseRevision {
			conflict := apitypes.ConflictResponse{}
			if exists {
				conflict.Current = &current
			}
			writeJSON(writer, http.StatusConflict, conflict)
			return
		}
		server.seq++
		sourceItems[itemID] = apitypes.Item{ID: itemID, Revision: moveRequest.Tombstone.Revision, Seq: server.seq, Deleted: true, Overview: moveRequest.Tombstone.Overview, Details: moveRequest.Tombstone.Details}
		server.seq++
		targetItems[itemID] = apitypes.Item{ID: itemID, Revision: moveRequest.Moved.Revision, Seq: server.seq, Overview: moveRequest.Moved.Overview, Details: moveRequest.Moved.Details}
		writeJSON(writer, http.StatusOK, apitypes.MoveItemResponse{SourceSeq: server.seq - 1, TargetSeq: server.seq})
	})
	mux.HandleFunc("GET /api/v1/events", func(writer http.ResponseWriter, request *http.Request) {
		if server.refusesSuspended(writer) {
			return
		}
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
	mux.HandleFunc("POST /api/v1/auth/logout", func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/v1/push/config", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if server.isPushUnsupported {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(writer, http.StatusOK, apitypes.PushConfig{RelayURL: server.pushRelayURL, AcceptsPayloadKey: server.acceptsPayloadKey})
	})
	mux.HandleFunc("PUT /api/v1/devices/current/push", func(writer http.ResponseWriter, request *http.Request) {
		var registration apitypes.PushRegistration
		json.NewDecoder(request.Body).Decode(&registration)
		server.mutex.Lock()
		server.pushRegistration = &registration
		server.mutex.Unlock()
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v1/devices/current/push", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		server.pushRegistration = nil
		server.mutex.Unlock()
		writer.WriteHeader(http.StatusNoContent)
	})
	server.addSharingRoutes(mux)
	server.addApprovalRoutes(mux)
	server.addVaultManagementRoutes(mux)
	server.addOrganizationRoutes(mux)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		isUnreachable, isSessionEnded := server.isUnreachable, server.isSessionEnded
		server.mutex.Unlock()
		if isUnreachable {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		if isSessionEnded {
			writeJSON(writer, http.StatusUnauthorized, apitypes.Error{Error: "unauthorized"})
			return
		}
		mux.ServeHTTP(writer, request)
	})
}
