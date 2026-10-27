// Command hisar-ui-test-mac plays the user's Mac for the Android phone approval UI test: it signs up from an invite,
// writes the account's Secret Key to a file for the phone to sign in with, then sends the phone an SSH request and a
// command-line request, one after the other. It verifies each answer as Hisar on a Mac does and saves the answer in the
// Personal vault ("SSH request: approve, remembered"), so the phone's test can read what the Mac received.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
)

type scriptedRequest struct {
	title       string
	description approval.Description
}

var scriptedRequests = []scriptedRequest{
	{"SSH request", approval.Description{
		Kind: approval.KindSSH, AppName: "Terminal", Purpose: "push to git.tryhisar.com", Host: "git.tryhisar.com:22",
		CommandLine: "git push origin main", KeyName: "hisar gitlab", RememberLabel: "Approve SSH requests from Terminal with this key for up to 1 hour. Any host. Ends when Hisar locks.", IsRememberedByDefault: true,
	}},
	{"Command-line request", approval.Description{
		Kind: approval.KindCommandLine, AppName: "hisar in Terminal", Purpose: "use Hisar",
		Allowance:   "Read secrets and one-time codes\nAdd, change and delete items\nManage vaults, sharing, invitations, organizations and devices\nShare links and exports still ask each time.",
		CommandLine: "/Applications/Hisar.app/Contents/Helpers/hisar", TerminalName: "ttys003",
	}},
}

func main() {
	invite := flag.String("invite", "", "invite link the Mac signs up with (required)")
	email := flag.String("email", "", "the account's email (required)")
	password := flag.String("password", "", "the account's master password (required)")
	stateDirectory := flag.String("state", "", "directory for the Mac's state and cache (required)")
	secretKeyPath := flag.String("secret-key-file", "", "file the account's Secret Key is written to (required)")
	flag.Parse()
	if *invite == "" || *email == "" || *password == "" || *stateDirectory == "" || *secretKeyPath == "" {
		log.Fatal("-invite, -email, -password, -state and -secret-key-file are required")
	}
	if err := os.MkdirAll(*stateDirectory, 0o700); err != nil {
		log.Fatal(err)
	}
	mac, err := app.NewService(app.Config{
		StatePath:      filepath.Join(*stateDirectory, "desktop.json"),
		CachePath:      filepath.Join(*stateDirectory, "desktop-cache.db"),
		DeviceName:     "UI Test Mac.local",
		Platform:       "darwin/desktop",
		SecretKeyStore: &memorySecretKeyStore{byAccountID: map[string]crypt.SecretKey{}, recoveryKeyByOperationID: map[string][]byte{}},
	})
	if err != nil {
		log.Fatal(err)
	}
	phoneApprover := app.NewPhoneApprover(mac)
	signUp, err := mac.SignUp(*invite, *email, "UI Test Mac", *password)
	if err != nil {
		log.Fatal(err)
	}
	if err := finishSignUp(mac, signUp, *password, *stateDirectory); err != nil {
		log.Fatal(err)
	}
	// Written whole, then renamed, so the script never reads half a key.
	temporaryPath := *secretKeyPath + ".tmp"
	if err := os.WriteFile(temporaryPath, []byte(signUp.SecretKey), 0o600); err != nil {
		log.Fatal(err)
	}
	if err := os.Rename(temporaryPath, *secretKeyPath); err != nil {
		log.Fatal(err)
	}
	log.Printf("signed up %s", *email)
	if err := waitForPhone(mac, *password); err != nil {
		log.Fatal(err)
	}
	for _, request := range scriptedRequests {
		answer := askPhone(phoneApprover, request.description)
		decision := answer.Decision
		if answer.IsRemembered {
			decision += ", remembered"
		}
		title := request.title + ": " + decision
		if err := saveToPersonalVault(mac, title); err != nil {
			log.Fatal(err)
		}
		log.Printf("saved %q", title)
	}
	log.Print("every request was answered")
	// The harness stops this Mac when the tests end. Waiting on a bare select would crash once auto-lock left nothing else
	// running ("all goroutines are asleep").
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}

// finishSignUp saves the new account's Emergency Kit and confirms it, as its user does in Hisar on a Mac; until then the
// account's setup stays pending on this Mac.
func finishSignUp(mac *app.Service, signUp app.SignUpResult, password, directory string) error {
	if signUp.SetupState != "delivery-pending" {
		return fmt.Errorf("sign-up stopped at %s: %s", signUp.SetupState, strings.Join(signUp.Problems, " "))
	}
	if _, err := app.ExportPendingRecoveryEmergencyKitToDirectory(mac, signUp.PendingOperationID, password, directory); err != nil {
		return err
	}
	return mac.AcknowledgePendingRecovery(signUp.PendingOperationID, password)
}

// waitForPhone waits for the phone's sign-in, then locks and unlocks: the Mac learned at sign-up that the account had no
// phone, and Hisar on a Mac checks that again only every few minutes or at the next unlock.
func waitForPhone(mac *app.Service, password string) error {
	for {
		devices, err := mac.ListDevices()
		if err != nil {
			return err
		}
		if slices.ContainsFunc(devices, func(device app.Device) bool { return apitypes.IsPhonePlatform(device.Platform) }) {
			break
		}
		time.Sleep(time.Second)
	}
	mac.Lock()
	if _, err := mac.Unlock(password); err != nil {
		return err
	}
	log.Print("the phone signed in")
	return nil
}

// askPhone waits for a phone on the account, then sends the request until the phone answers it, asking again when one
// expires unanswered.
func askPhone(phoneApprover *app.PhoneApprover, description approval.Description) approval.AcceptedAnswer {
	for {
		for !phoneApprover.IsPhoneApprovalAvailable() {
			time.Sleep(time.Second)
		}
		answers := make(chan approval.AcceptedAnswer, 1)
		expiresAt := time.Now().Add(approval.MaximumLifetime)
		finish, err := phoneApprover.StartPhoneApproval(description, expiresAt, func(answer approval.AcceptedAnswer) { answers <- answer })
		if err != nil {
			log.Printf("could not ask the phone: %v", err)
			time.Sleep(time.Second)
			continue
		}
		log.Printf("asked the phone: %s wants to %s", description.AppName, description.Purpose)
		select {
		case answer := <-answers:
			finish()
			return answer
		case <-time.After(time.Until(expiresAt)):
			finish()
			log.Print("the request expired unanswered; asking again")
		}
	}
}

func saveToPersonalVault(mac *app.Service, title string) error {
	vaults, err := mac.ListVaults()
	if err != nil {
		return err
	}
	for _, vault := range vaults {
		if vault.Type == "personal" {
			_, err := mac.SaveItem(vault.ID, app.ItemDetail{Category: item.CategorySecureNote, Title: title})
			return err
		}
	}
	return errors.New("the account has no Personal vault")
}

type memorySecretKeyStore struct {
	mutex                    sync.Mutex
	byAccountID              map[string]crypt.SecretKey
	recoveryKeyByOperationID map[string][]byte
}

func (store *memorySecretKeyStore) LoadSecretKey(accountID string) (crypt.SecretKey, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	secretKey, isFound := store.byAccountID[accountID]
	if !isFound {
		return crypt.SecretKey{}, errors.New("not in the keychain")
	}
	return secretKey, nil
}

func (store *memorySecretKeyStore) StoreSecretKey(secretKey crypt.SecretKey) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.byAccountID[secretKey.AccountID] = secretKey
	return nil
}

func (store *memorySecretKeyStore) DeleteSecretKey(accountID string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.byAccountID, accountID)
	return nil
}

// The recovery keys make the store a keychain to sign-up, which otherwise stops before registering the account.
func (store *memorySecretKeyStore) StoreRecoveryKey(operationID string, key []byte) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.recoveryKeyByOperationID[operationID] = slices.Clone(key)
	return nil
}

func (store *memorySecretKeyStore) LoadRecoveryKey(operationID string) ([]byte, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	key, isFound := store.recoveryKeyByOperationID[operationID]
	if !isFound {
		return nil, errors.New("not in the keychain")
	}
	return slices.Clone(key), nil
}

func (store *memorySecretKeyStore) DeleteRecoveryKey(operationID string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.recoveryKeyByOperationID, operationID)
	return nil
}
