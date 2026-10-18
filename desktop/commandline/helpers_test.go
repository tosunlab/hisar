package commandline

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

const testOneTimePassword = "otpauth://totp/Example:alice?secret=JBSWY3DPEHPK3PXP&issuer=Example"

func createRaycastRequester(signature appinfo.Signature) appinfo.Requester {
	return appinfo.Requester{
		Kind: appinfo.RequesterKindApp,
		Name: "Raycast",
		App:  appinfo.App{Name: "Raycast", Path: testRaycastPath, Signature: signature},
	}
}

type staticPeerResolver struct{}

func (staticPeerResolver) ResolvePeer(net.Conn) (clisocket.Peer, error) {
	return clisocket.Peer{UID: os.Getuid(), PID: 4242}, nil
}

func (staticPeerResolver) GetTerminalSession(int) (clisocket.TerminalSession, error) {
	return clisocket.TerminalSession{SessionID: 4200, TerminalDevice: 7, LeaderStartTime: 1}, nil
}

func (staticPeerResolver) GetProcessPath(int) (string, error) { return "/usr/local/bin/hisar", nil }

func (staticPeerResolver) GetParentPID(int) (int, error) { return 4000, nil }

type countingApprover struct {
	mutex sync.Mutex
	count int
}

func (approver *countingApprover) ApproveCommandLine(context.Context, clisocket.ApprovalRequest) error {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.count++
	return nil
}

// savingSecretKeyStore keeps Secret Keys in memory, for tests that sign up.
type savingSecretKeyStore struct {
	mutex      sync.Mutex
	secretKeys map[string]crypt.SecretKey
}

func (store *savingSecretKeyStore) LoadSecretKey(accountID string) (crypt.SecretKey, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	secretKey, found := store.secretKeys[accountID]
	if !found {
		return crypt.SecretKey{}, os.ErrNotExist
	}
	return secretKey, nil
}

func (store *savingSecretKeyStore) StoreSecretKey(secretKey crypt.SecretKey) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.secretKeys[secretKey.AccountID] = secretKey
	return nil
}

func (store *savingSecretKeyStore) DeleteSecretKey(accountID string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.secretKeys, accountID)
	return nil
}

func createSignedOutAppService(t *testing.T) *app.Service {
	t.Helper()
	directory := t.TempDir()
	service, err := app.NewService(app.Config{
		StatePath: filepath.Join(directory, "desktop.json"), CachePath: filepath.Join(directory, "desktop-cache.db"),
		DeviceName: "test", Platform: "test/desktop", SecretKeyStore: &savingSecretKeyStore{secretKeys: map[string]crypt.SecretKey{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Lock() })
	return service
}

func findPersonalVaultID(t *testing.T, service *app.Service) string {
	t.Helper()
	vaults, err := service.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, vault := range vaults {
		if vault.Type == "personal" {
			return vault.ID
		}
	}
	t.Fatal("no personal vault")
	return ""
}
