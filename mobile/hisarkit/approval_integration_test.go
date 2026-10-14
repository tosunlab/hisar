package hisarkit

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	sshAgentClient "golang.org/x/crypto/ssh/agent"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localsocket"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/agent"
	"github.com/tryhisar/hisar/desktop/vault"
)

type approvalEventRecorder struct {
	allEvent chan [2]string
}

func (recorder *approvalEventRecorder) OnLocked() {}

func (recorder *approvalEventRecorder) OnRemoteChange() {}

func (recorder *approvalEventRecorder) OnUpdateRequired() {}

func (recorder *approvalEventRecorder) OnApprovalEvent(eventName, requestID string) {
	recorder.allEvent <- [2]string{eventName, requestID}
}

func (recorder *approvalEventRecorder) waitFor(t *testing.T, eventName, requestID string) {
	t.Helper()
	timeout := time.After(15 * time.Second)
	for {
		select {
		case event := <-recorder.allEvent:
			if event[0] == eventName && (requestID == "" || event[1] == requestID) {
				return
			}
		case <-timeout:
			t.Fatalf("the phone never heard %s", eventName)
		}
	}
}

type macSecretKeyStore struct {
	mutex          sync.Mutex
	allByAccountID map[string]crypt.SecretKey
}

func (store *macSecretKeyStore) LoadSecretKey(accountID string) (crypt.SecretKey, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	secretKey, isFound := store.allByAccountID[accountID]
	if !isFound {
		return crypt.SecretKey{}, errors.New("not in the keychain")
	}
	return secretKey, nil
}

func (store *macSecretKeyStore) StoreSecretKey(secretKey crypt.SecretKey) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.allByAccountID[secretKey.AccountID] = secretKey
	return nil
}

func (store *macSecretKeyStore) DeleteSecretKey(accountID string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.allByAccountID, accountID)
	return nil
}

type macDialog struct {
	allEvent chan agent.ApprovalEvent
}

func (dialog *macDialog) emit(eventName string, data any) {
	if event, isApprovalEvent := data.(agent.ApprovalEvent); isApprovalEvent && eventName == agent.ApprovalEventName {
		dialog.allEvent <- event
	}
}

func (dialog *macDialog) waitForEvent(t *testing.T) agent.ApprovalEvent {
	t.Helper()
	select {
	case event := <-dialog.allEvent:
		return event
	case <-time.After(15 * time.Second):
		t.Fatal("the Mac never showed the approval dialog")
		return agent.ApprovalEvent{}
	}
}

type phoneApprovalRequest struct {
	RequestID           string `json:"requestId"`
	RequesterDeviceName string `json:"requesterDeviceName"`
	ExpiresAt           string `json:"expiresAt"`
	Kind                string `json:"kind"`
	AppName             string `json:"appName"`
	Purpose             string `json:"purpose"`
	KeyName             string `json:"keyName"`
	IsForwarded         bool   `json:"isForwarded"`
	RememberLabel       string `json:"rememberLabel"`
}

func signInWithSSHAgent(socketPath, address string, hostKey ssh.PublicKey) error {
	agentConnection, err := net.Dial("unix", socketPath)
	if err != nil {
		return err
	}
	defer agentConnection.Close()
	agentClient := sshAgentClient.NewClient(agentConnection)
	sshClient, err := ssh.Dial("tcp", address, &ssh.ClientConfig{
		User:            "git",
		Auth:            []ssh.AuthMethod{ssh.PublicKeysCallback(agentClient.Signers)},
		HostKeyCallback: ssh.FixedHostKey(hostKey),
		Timeout:         30 * time.Second,
	})
	if err != nil {
		return err
	}
	return sshClient.Close()
}

func startSSHServer(t *testing.T, authorizedKey ssh.PublicKey) (string, ssh.PublicKey) {
	t.Helper()
	_, hostPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(metadata ssh.ConnMetadata, publicKey ssh.PublicKey) (*ssh.Permissions, error) {
			if string(publicKey.Marshal()) == string(authorizedKey.Marshal()) {
				return &ssh.Permissions{}, nil
			}
			return nil, errors.New("unknown public key")
		},
	}
	serverConfig.AddHostKey(hostSigner)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				serverConnection, allNewChannel, allRequest, err := ssh.NewServerConn(connection, serverConfig)
				if err != nil {
					return
				}
				defer serverConnection.Close()
				go ssh.DiscardRequests(allRequest)
				for newChannel := range allNewChannel {
					newChannel.Reject(ssh.Prohibited, "no channels")
				}
			}()
		}
	}()
	return listener.Addr().String(), hostSigner.PublicKey()
}

// TestIntegrationPhoneApproval signs in a Mac backend and an iPhone backend to one account: the phone approves
// and denies the Mac's SSH requests, and the Mac refuses an answer it cannot verify.
func TestIntegrationPhoneApproval(t *testing.T) {
	inviteLink := os.Getenv("HISAR_TEST_PHONE_APPROVAL_INVITE_LINK")
	if inviteLink == "" {
		t.Skip("HISAR_TEST_PHONE_APPROVAL_INVITE_LINK not set")
	}
	parsedInvite, err := url.Parse(inviteLink)
	if err != nil {
		t.Fatal(err)
	}
	serverURL := parsedInvite.Scheme + "://" + parsedInvite.Host
	const email, password = "phone-approval@example.com", "phone approval master password"

	phoneDirectory := t.TempDir()
	recorder := &approvalEventRecorder{allEvent: make(chan [2]string, 64)}
	biometric := &fakeBiometric{allKeyByAccountID: map[string][]byte{}}
	phoneSecretKeyStore := createMemorySecretKeyStore()
	phone, err := New(phoneDirectory, "Test iPhone", phoneSecretKeyStore, biometric, &fakeClipboard{}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	signUp := decodeJSON[struct {
		Status    status `json:"status"`
		SecretKey string `json:"secretKey"`
	}](t)(phone.SignUp(inviteLink, email, "Phone Approval", password))
	if signUp.Status.State != "unlocked" {
		t.Fatalf("phone sign up: %+v", signUp)
	}

	macDirectory := t.TempDir()
	mac, err := vault.CreateService(vault.Config{
		StatePath:      filepath.Join(macDirectory, "desktop.json"),
		CachePath:      filepath.Join(macDirectory, "desktop-cache.db"),
		DeviceName:     "Test MacBook Pro.local",
		Platform:       "darwin/desktop",
		SecretKeyStore: &macSecretKeyStore{allByAccountID: map[string]crypt.SecretKey{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	phoneApprover := vault.CreatePhoneApprover(mac)
	if _, err := mac.SignIn(serverURL, email, signUp.SecretKey, password); err != nil {
		t.Fatal(err)
	}
	defer mac.SignOut()
	deadline := time.Now().Add(15 * time.Second)
	for !phoneApprover.IsPhoneApprovalAvailable() {
		if time.Now().After(deadline) {
			t.Fatal("the Mac never saw the iPhone on the account")
		}
		time.Sleep(50 * time.Millisecond)
	}

	allVault, err := mac.ListAllVault()
	if err != nil || len(allVault) == 0 {
		t.Fatalf("vaults: %+v %v", allVault, err)
	}
	generated, err := mac.GenerateSSHKey(allVault[0].ID, "Phone approval key", "")
	if err != nil {
		t.Fatal(err)
	}
	authorizedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(generated.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	dialog := &macDialog{allEvent: make(chan agent.ApprovalEvent, 8)}
	approver := agent.CreateApprover(agent.ApproverConfig{Emit: dialog.emit, PhoneApprover: phoneApprover, Timeout: 30 * time.Second})
	agentServer := sshagent.CreateServer(vault.CreateSSHKeySource(mac), approver, localsocket.SystemPeerResolver{})
	socketDirectory, err := os.MkdirTemp("", "hisar")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(socketDirectory)
	socketPath := filepath.Join(socketDirectory, "agent.sock")
	listener, err := localsocket.ListenUnixSocket(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	agentContext, stopAgent := context.WithCancel(context.Background())
	defer stopAgent()
	go agentServer.Serve(agentContext, listener)
	address, hostKey := startSSHServer(t, authorizedKey)

	startSSHSignIn := func() chan error {
		agentServer.ClearAllApproval()
		resultChannel := make(chan error, 1)
		go func() { resultChannel <- signInWithSSHAgent(socketPath, address, hostKey) }()
		return resultChannel
	}
	waitForSSHSignIn := func(resultChannel chan error) error {
		select {
		case err := <-resultChannel:
			return err
		case <-time.After(45 * time.Second):
			t.Fatal("the SSH sign-in never finished")
			return nil
		}
	}
	findPhoneRequest := func(requestID string) phoneApprovalRequest {
		t.Helper()
		allRequest := decodeJSON[[]phoneApprovalRequest](t)(phone.ListAllPendingApproval())
		for _, request := range allRequest {
			if request.RequestID == requestID {
				return request
			}
		}
		t.Fatalf("the phone does not list %s: %+v", requestID, allRequest)
		return phoneApprovalRequest{}
	}

	resultChannel := startSSHSignIn()
	dialogEvent := dialog.waitForEvent(t)
	if !dialogEvent.IsPhoneApproval {
		t.Fatalf("the Mac's dialog does not offer the iPhone: %+v", dialogEvent)
	}
	recorder.waitFor(t, "approval-requested", "")
	allRequest := decodeJSON[[]phoneApprovalRequest](t)(phone.ListAllPendingApproval())
	if len(allRequest) != 1 || allRequest[0].RequesterDeviceName != "Test MacBook Pro" || allRequest[0].Kind != approval.KindSSH ||
		allRequest[0].KeyName != "Phone approval key" || allRequest[0].IsForwarded || allRequest[0].Purpose == "" {
		t.Fatalf("the phone shows %+v", allRequest)
	}
	requestID := allRequest[0].RequestID
	if err := phone.ApproveApproval(requestID, false); err != nil {
		t.Fatal(err)
	}
	if err := waitForSSHSignIn(resultChannel); err != nil {
		t.Fatalf("the SSH sign-in the phone approved failed: %v", err)
	}
	if biometric.authenticateCount != 1 {
		t.Errorf("Face ID was asked %d times for one approval", biometric.authenticateCount)
	}
	recorder.waitFor(t, "approval-resolved", requestID)
	if err := phone.DenyApproval(requestID); err == nil {
		t.Error("answered a request twice")
	}

	startSSHSignInKeepingApproval := func() chan error {
		resultChannel := make(chan error, 1)
		go func() { resultChannel <- signInWithSSHAgent(socketPath, address, hostKey) }()
		return resultChannel
	}
	resultChannel = startSSHSignInKeepingApproval()
	dialog.waitForEvent(t)
	recorder.waitFor(t, "approval-requested", "")
	rememberedRequest := decodeJSON[[]phoneApprovalRequest](t)(phone.ListAllPendingApproval())[0]
	if rememberedRequest.RememberLabel == "" {
		t.Fatalf("the phone is not told that approving also allows the app for an hour: %+v", rememberedRequest)
	}
	if err := phone.ApproveApproval(rememberedRequest.RequestID, true); err != nil {
		t.Fatal(err)
	}
	if err := waitForSSHSignIn(resultChannel); err != nil {
		t.Fatalf("the SSH sign-in the phone approved for an hour failed: %v", err)
	}
	recorder.waitFor(t, "approval-resolved", rememberedRequest.RequestID)
	if err := waitForSSHSignIn(startSSHSignInKeepingApproval()); err != nil {
		t.Errorf("the phone's choice to allow the app for an hour was not remembered: %v", err)
	}

	resultChannel = startSSHSignIn()
	dialog.waitForEvent(t)
	recorder.waitFor(t, "approval-requested", "")
	deniedID := decodeJSON[[]phoneApprovalRequest](t)(phone.ListAllPendingApproval())[0].RequestID
	if err := phone.DenyApproval(deniedID); err != nil {
		t.Fatal(err)
	}
	if err := waitForSSHSignIn(resultChannel); err == nil {
		t.Error("the SSH sign-in the phone denied succeeded")
	}
	if biometric.authenticateCount != 2 {
		t.Errorf("Deny asked for Face ID")
	}

	resultChannel = startSSHSignIn()
	forgedDialog := dialog.waitForEvent(t)
	recorder.waitFor(t, "approval-requested", "")
	forgedID := decodeJSON[[]phoneApprovalRequest](t)(phone.ListAllPendingApproval())[0].RequestID
	findPhoneRequest(forgedID)
	phoneState, err := localstate.Load(filepath.Join(phoneDirectory, "hisar.json"))
	if err != nil {
		t.Fatal(err)
	}
	forger := apiclient.Create(serverURL)
	forger.Token = phoneState.Token
	ctx := context.Background()
	account, err := forger.GetAccount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := forger.GetApproval(ctx, forgedID)
	if err != nil {
		t.Fatal(err)
	}
	otherKeyset, _ := crypt.GenerateKeyset()
	fields := crypt.ApprovalAnswerFields{
		AccountID: account.AccountID, RequestID: forgedID, RequestHash: crypt.HashApprovalRequest(stored.SealedRequest),
		Decision: approval.DecisionApprove, AnsweringDeviceID: "forged-device", AnsweringDeviceName: "Forged iPhone", AnsweredAt: approval.FormatTime(time.Now()),
	}
	plaintext, _ := json.Marshal(approval.Answer{
		RequestID: forgedID, Decision: fields.Decision, AnsweringDeviceID: fields.AnsweringDeviceID, AnsweringDeviceName: fields.AnsweringDeviceName,
		AnsweredAt: fields.AnsweredAt, RequestHash: fields.RequestHash, Signature: otherKeyset.SignApprovalAnswer(fields),
	})
	forgedAnswer, err := crypt.SealApprovalAnswer(account.PublicKeys.KEM, account.AccountID, forgedID, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if err := forger.AnswerApproval(ctx, forgedID, forgedAnswer); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-resultChannel:
		t.Fatalf("the SSH sign-in finished after a forged answer: %v", err)
	case <-time.After(2 * time.Second):
	}
	if !approver.Respond(forgedDialog.RequestID, false, false) {
		t.Error("the dialog closed after a forged answer")
	}
	if err := waitForSSHSignIn(resultChannel); err == nil {
		t.Error("the SSH sign-in succeeded with a forged answer")
	}

	// The app is not running when the Mac asks: it starts later, unlocks, and finds the request still waiting.
	phone.Lock()
	resultChannel = startSSHSignIn()
	lateDialog := dialog.waitForEvent(t)
	if !lateDialog.IsPhoneApproval {
		t.Fatalf("the Mac did not send the request while the phone app was closed: %+v", lateDialog)
	}
	time.Sleep(2 * time.Second)
	relaunchedPhone, err := New(phoneDirectory, "Test iPhone", phoneSecretKeyStore, biometric, &fakeClipboard{}, &approvalEventRecorder{allEvent: make(chan [2]string, 64)})
	if err != nil {
		t.Fatal(err)
	}
	if current := decodeJSON[status](t)(relaunchedPhone.Unlock(password)); current.State != "unlocked" {
		t.Fatalf("relaunched phone: %+v", current)
	}
	lateRequests := decodeJSON[[]phoneApprovalRequest](t)(relaunchedPhone.ListAllPendingApproval())
	if len(lateRequests) != 1 {
		t.Fatalf("the phone that opened later sees %+v", lateRequests)
	}
	if expiresAt, err := time.Parse(time.RFC3339Nano, lateRequests[0].ExpiresAt); err != nil || time.Until(expiresAt) < time.Minute {
		t.Errorf("the request expires at %q, before the Mac's dialog gives up", lateRequests[0].ExpiresAt)
	}
	if err := relaunchedPhone.ApproveApproval(lateRequests[0].RequestID, false); err != nil {
		t.Fatal(err)
	}
	if err := waitForSSHSignIn(resultChannel); err != nil {
		t.Fatalf("the SSH sign-in approved by the phone that opened later failed: %v", err)
	}

	mac.Lock()
	if phoneApprover.IsPhoneApprovalAvailable() {
		t.Error("a locked Mac offers phone approvals")
	}
	if allRequest := decodeJSON[[]phoneApprovalRequest](t)(relaunchedPhone.ListAllPendingApproval()); len(allRequest) != 0 {
		t.Errorf("requests left on the phone: %+v", allRequest)
	}
}
