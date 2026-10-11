package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/pake"
	"github.com/tryhisar/hisar/core/proto"
)

var (
	ErrBadCredentials = errors.New("incorrect email, password or Secret Key")
	ErrConflict       = errors.New("item was changed on another device")
	ErrRollback       = errors.New("server returned older vault state than previously seen")
	ErrKeyMismatch    = errors.New("server returned public keys that do not match the keyset")
	ErrNotFound       = errors.New("not found")
)

// SeqStore remembers the highest vault seq seen, to detect server rollbacks.
type SeqStore interface {
	GetSeq(vaultID string) int64
	SetSeq(vaultID string, seq int64)
}

type Session struct {
	Client    *client.Client
	AccountID string
	Email     string
	Name      string
	Role      string
	Keyset    *crypt.Keyset
	SeqStore  SeqStore
	// MemberVerifier, when set, is asked to accept each member a vault key is wrapped for (e.g. key pinning).
	MemberVerifier func(member *proto.Member) error
	// Cache, when set, stores the server's encrypted records; reads fall back to it while the server is unreachable.
	Cache     Cache
	isOffline atomic.Bool
}

type Vault struct {
	ID   string
	Type string
	Name string
	Role string
	Key  []byte
}

func (s *Session) Lock() {
	if s.Keyset != nil {
		s.Keyset.Wipe()
		s.Keyset = nil
	}
}

// accountCredential is what signup generates: a new Secret Key, salt, keyset and OPAQUE record.
type accountCredential struct {
	secretKey        crypt.SecretKey
	salt             []byte
	argon2Params     crypt.Argon2Params
	keyset           *crypt.Keyset
	encryptedKeyset  []byte
	record           []byte
	accountUnlockKey []byte
}

// createAccountCredential runs OPAQUE registration through register, which sends the request and returns the server's response.
func createAccountCredential(accountID string, password []byte, register func(registrationRequest []byte) ([]byte, error)) (*accountCredential, error) {
	secretKey, err := crypt.GenerateSecretKey(accountID)
	if err != nil {
		return nil, err
	}
	credential := &accountCredential{secretKey: secretKey, salt: crypt.GenerateSalt(), argon2Params: crypt.DefaultArgon2Params}
	derivedKeys, err := crypt.DeriveAllKey(password, credential.salt, credential.argon2Params, secretKey)
	if err != nil {
		return nil, err
	}
	defer derivedKeys.Wipe()

	if credential.keyset, err = crypt.GenerateKeyset(); err != nil {
		return nil, err
	}
	if err := credential.registerOpaque(derivedKeys, accountID, register); err != nil {
		credential.keyset.Wipe()
		return nil, err
	}
	credential.accountUnlockKey = append([]byte(nil), derivedKeys.AccountUnlockKey...)
	return credential, nil
}

func (credential *accountCredential) registerOpaque(derivedKeys *crypt.DerivedKeys, accountID string, register func([]byte) ([]byte, error)) error {
	var err error
	if credential.encryptedKeyset, err = credential.keyset.Encrypt(derivedKeys.AccountUnlockKey, accountID); err != nil {
		return err
	}
	opaqueClient, err := pake.CreateConfig().Client()
	if err != nil {
		return err
	}
	defer opaqueClient.ClearState()
	registrationRequest, err := opaqueClient.RegistrationInit(derivedKeys.AuthKey)
	if err != nil {
		return err
	}
	encodedResponse, err := register(registrationRequest.Serialize())
	if err != nil {
		return err
	}
	registrationResponse, err := opaqueClient.Deserialize.RegistrationResponse(encodedResponse)
	if err != nil {
		return err
	}
	record, _, err := opaqueClient.RegistrationFinalize(registrationResponse, []byte(accountID), pake.ServerIdentity)
	if err != nil {
		return err
	}
	credential.record = record.Serialize()
	return nil
}

func SignUp(ctx context.Context, apiClient *client.Client, invite, email, name string, password []byte, device proto.DeviceInfo) (*Session, crypt.SecretKey, error) {
	newSession, secretKey, accountUnlockKey, err := SignUpKeepingAccountUnlockKey(ctx, apiClient, invite, email, name, password, device)
	clear(accountUnlockKey)
	return newSession, secretKey, err
}

// SignUpKeepingAccountUnlockKey also returns a copy of the account unlock key; the caller wipes it.
func SignUpKeepingAccountUnlockKey(ctx context.Context, apiClient *client.Client, invite, email, name string, password []byte, device proto.DeviceInfo) (*Session, crypt.SecretKey, []byte, error) {
	accountID := crypt.GenerateAccountID()
	credential, err := createAccountCredential(accountID, password, func(registrationRequest []byte) ([]byte, error) {
		startResponse, err := apiClient.StartRegistration(ctx, &proto.RegisterStartRequest{Invite: invite, Email: email, AccountID: accountID, Request: registrationRequest})
		if err != nil {
			return nil, err
		}
		return startResponse.Response, nil
	})
	if err != nil {
		return nil, crypt.SecretKey{}, nil, err
	}
	secretKey := credential.secretKey

	sessionResponse, err := apiClient.FinishRegistration(ctx, &proto.RegisterFinishRequest{
		Invite:     invite,
		Email:      email,
		Name:       name,
		AccountID:  accountID,
		Record:     credential.record,
		Salt:       credential.salt,
		Argon2:     credential.argon2Params,
		EncKeyset:  credential.encryptedKeyset,
		PublicKeys: credential.keyset.GetAllPublicKey(accountID),
		Device:     device,
	})
	if err != nil {
		credential.keyset.Wipe()
		clear(credential.accountUnlockKey)
		return nil, secretKey, nil, err
	}
	apiClient.Token = sessionResponse.Token

	newSession := &Session{Client: apiClient, AccountID: accountID, Email: email, Name: name, Keyset: credential.keyset}
	if _, err := newSession.CreateVault(ctx, "Personal", proto.VaultPersonal); err != nil {
		clear(credential.accountUnlockKey)
		return nil, secretKey, nil, fmt.Errorf("create personal vault: %w", err)
	}
	return newSession, secretKey, credential.accountUnlockKey, nil
}

func LogIn(ctx context.Context, apiClient *client.Client, email string, password []byte, secretKey crypt.SecretKey, device proto.DeviceInfo) (*Session, error) {
	prelogin, err := apiClient.FetchPrelogin(ctx, email)
	if err != nil {
		return nil, err
	}
	derivedKeys, err := crypt.DeriveAllKey(password, prelogin.Salt, prelogin.Argon2, secretKey)
	if err != nil {
		return nil, err
	}
	defer derivedKeys.Wipe()

	opaqueClient, err := pake.CreateConfig().Client()
	if err != nil {
		return nil, err
	}
	keyExchange1, err := opaqueClient.GenerateKE1(derivedKeys.AuthKey)
	if err != nil {
		return nil, err
	}
	startResponse, err := apiClient.StartLogin(ctx, &proto.LoginStartRequest{Email: email, KE1: keyExchange1.Serialize()})
	if err != nil {
		return nil, err
	}
	keyExchange2, err := opaqueClient.Deserialize.KE2(startResponse.KE2)
	if err != nil {
		return nil, ErrBadCredentials
	}
	keyExchange3, _, _, err := opaqueClient.GenerateKE3(keyExchange2, []byte(secretKey.AccountID), pake.ServerIdentity)
	opaqueClient.ClearState()
	if err != nil {
		return nil, ErrBadCredentials
	}
	sessionResponse, err := apiClient.FinishLogin(ctx, &proto.LoginFinishRequest{LoginID: startResponse.LoginID, KE3: keyExchange3.Serialize(), Device: device})
	if client.IsStatus(err, http.StatusUnauthorized) {
		return nil, ErrBadCredentials
	}
	if err != nil {
		return nil, err
	}
	apiClient.Token = sessionResponse.Token
	if sessionResponse.AccountID != secretKey.AccountID {
		return nil, ErrBadCredentials
	}
	return UnlockWithAccountUnlockKey(ctx, apiClient, nil, derivedKeys.AccountUnlockKey, secretKey.AccountID)
}

// Unlock decrypts the keyset; with a non-nil accountCache it works offline from the cached account.
func Unlock(ctx context.Context, apiClient *client.Client, accountCache Cache, password []byte, secretKey crypt.SecretKey) (*Session, error) {
	account, isOffline, err := getAccount(ctx, apiClient, accountCache, secretKey.AccountID)
	if err != nil {
		return nil, err
	}
	if account.AccountID != secretKey.AccountID {
		return nil, ErrBadCredentials
	}
	derivedKeys, err := crypt.DeriveAllKey(password, account.Salt, account.Argon2, secretKey)
	if err != nil {
		return nil, err
	}
	defer derivedKeys.Wipe()
	return decryptAccountKeyset(account, apiClient, accountCache, isOffline, derivedKeys.AccountUnlockKey)
}

// DeriveAccountUnlockKey returns a copy of the account unlock key after checking it opens the keyset; the caller wipes it.
func DeriveAccountUnlockKey(ctx context.Context, apiClient *client.Client, accountCache Cache, password []byte, secretKey crypt.SecretKey) ([]byte, error) {
	account, _, err := getAccount(ctx, apiClient, accountCache, secretKey.AccountID)
	if err != nil {
		return nil, err
	}
	if account.AccountID != secretKey.AccountID {
		return nil, ErrBadCredentials
	}
	derivedKeys, err := crypt.DeriveAllKey(password, account.Salt, account.Argon2, secretKey)
	if err != nil {
		return nil, err
	}
	defer derivedKeys.Wipe()
	keyset, err := crypt.DecryptKeyset(derivedKeys.AccountUnlockKey, account.AccountID, account.EncKeyset)
	if err != nil {
		return nil, ErrBadCredentials
	}
	keyset.Wipe()
	return append([]byte(nil), derivedKeys.AccountUnlockKey...), nil
}

// UnlockWithAccountUnlockKey decrypts the keyset with an already derived account unlock key.
func UnlockWithAccountUnlockKey(ctx context.Context, apiClient *client.Client, accountCache Cache, accountUnlockKey []byte, accountID string) (*Session, error) {
	account, isOffline, err := getAccount(ctx, apiClient, accountCache, accountID)
	if err != nil {
		return nil, err
	}
	if account.AccountID != accountID {
		return nil, ErrBadCredentials
	}
	return decryptAccountKeyset(account, apiClient, accountCache, isOffline, accountUnlockKey)
}

func decryptAccountKeyset(account *proto.Account, apiClient *client.Client, accountCache Cache, isOffline bool, accountUnlockKey []byte) (*Session, error) {
	keyset, err := crypt.DecryptKeyset(accountUnlockKey, account.AccountID, account.EncKeyset)
	if err != nil {
		return nil, ErrBadCredentials
	}
	ownPublicKeyBundle := keyset.GetAllPublicKey(account.AccountID)
	if !bytes.Equal(ownPublicKeyBundle.KEM, account.PublicKeys.KEM) || !bytes.Equal(ownPublicKeyBundle.Sign, account.PublicKeys.Sign) {
		keyset.Wipe()
		return nil, ErrKeyMismatch
	}
	unlockedSession := &Session{Client: apiClient, AccountID: account.AccountID, Email: account.Email, Name: account.Name, Role: account.Role, Keyset: keyset, Cache: accountCache}
	unlockedSession.isOffline.Store(isOffline)
	return unlockedSession, nil
}

func (s *Session) CreateVault(ctx context.Context, name, vaultType string) (*Vault, error) {
	vaultID := uuid.NewString()
	vaultKey := crypt.GenerateKey()
	encryptedName, err := crypt.SealVaultName(vaultKey, vaultID, []byte(name))
	if err != nil {
		return nil, err
	}
	wrappedKey, err := crypt.WrapVaultKey(s.Keyset.GetAllPublicKey(s.AccountID).KEM, vaultID, s.AccountID, vaultKey)
	if err != nil {
		return nil, err
	}
	request := &proto.CreateVaultRequest{ID: vaultID, Type: vaultType, EncName: encryptedName, WrappedKey: wrappedKey}
	if err := s.Client.CreateVault(ctx, request); err != nil {
		return nil, err
	}
	return &Vault{ID: vaultID, Type: vaultType, Name: name, Role: proto.VaultOwner, Key: vaultKey}, nil
}

func (s *Session) ListAllVault(ctx context.Context) ([]Vault, error) {
	allProtoVault, err := s.listAllProtoVault(ctx)
	if err != nil {
		return nil, err
	}
	allVault := make([]Vault, 0, len(allProtoVault))
	for _, protoVault := range allProtoVault {
		vaultKey, err := s.Keyset.UnwrapVaultKey(protoVault.ID, s.AccountID, protoVault.WrappedKey)
		if err != nil {
			return nil, fmt.Errorf("vault %s: %w", protoVault.ID, err)
		}
		name, err := crypt.OpenVaultName(vaultKey, protoVault.ID, protoVault.EncName)
		if err != nil {
			return nil, fmt.Errorf("vault %s: %w", protoVault.ID, err)
		}
		allVault = append(allVault, Vault{
			ID: protoVault.ID, Type: protoVault.Type, Name: string(name), Role: protoVault.Role, Key: vaultKey,
		})
	}
	return allVault, nil
}

func (s *Session) FindVault(ctx context.Context, nameOrID string) (*Vault, error) {
	allVault, err := s.ListAllVault(ctx)
	if err != nil {
		return nil, err
	}
	for i := range allVault {
		if allVault[i].ID == nameOrID || strings.EqualFold(allVault[i].Name, nameOrID) {
			return &allVault[i], nil
		}
	}
	return nil, fmt.Errorf("vault %q: %w", nameOrID, ErrNotFound)
}

// ListAllItem fetches and decrypts all live items of a vault; with a Cache it downloads only the changes.
func (s *Session) ListAllItem(ctx context.Context, vault *Vault) ([]item.Item, error) {
	if s.Cache == nil {
		allProtoItem, err := s.downloadAllItem(ctx, vault)
		if err != nil {
			return nil, err
		}
		return decryptAllItem(vault, allProtoItem)
	}
	allProtoItem, err := s.syncVault(ctx, vault)
	if err != nil {
		return nil, err
	}
	allItem, err := decryptAllItem(vault, allProtoItem)
	if err == nil || s.IsOffline() {
		return allItem, err
	}
	// A cached item that no longer decrypts, e.g. after a key rotation raced a sync, is downloaded again.
	if err := s.Cache.ResetVault(vault.ID); err != nil {
		return nil, err
	}
	if allProtoItem, err = s.syncVault(ctx, vault); err != nil {
		return nil, err
	}
	return decryptAllItem(vault, allProtoItem)
}

func (s *Session) downloadAllItem(ctx context.Context, vault *Vault) ([]proto.Item, error) {
	var since, nextSeq int64
	allProtoItemByID := map[string]proto.Item{}
	for {
		changeResponse, err := s.Client.ListAllChange(ctx, vault.ID, since, 0)
		if err != nil {
			return nil, err
		}
		for _, protoItem := range changeResponse.Items {
			allProtoItemByID[protoItem.ID] = protoItem
		}
		nextSeq = changeResponse.NextSeq
		if !changeResponse.More {
			break
		}
		since = changeResponse.NextSeq
	}
	if s.SeqStore != nil {
		if nextSeq < s.SeqStore.GetSeq(vault.ID) {
			return nil, ErrRollback
		}
		s.SeqStore.SetSeq(vault.ID, nextSeq)
	}
	allProtoItem := make([]proto.Item, 0, len(allProtoItemByID))
	for _, protoItem := range allProtoItemByID {
		if !protoItem.Deleted {
			allProtoItem = append(allProtoItem, protoItem)
		}
	}
	return allProtoItem, nil
}

func decryptAllItem(vault *Vault, allProtoItem []proto.Item) ([]item.Item, error) {
	allItem := make([]item.Item, 0, len(allProtoItem))
	for _, protoItem := range allProtoItem {
		vaultItem, err := decryptItem(vault, &protoItem)
		if err != nil {
			return nil, err
		}
		allItem = append(allItem, *vaultItem)
	}
	sort.Slice(allItem, func(i, j int) bool {
		return strings.ToLower(allItem[i].Overview.Title) < strings.ToLower(allItem[j].Overview.Title)
	})
	return allItem, nil
}

func decryptItem(vault *Vault, protoItem *proto.Item) (*item.Item, error) {
	vaultItem := &item.Item{ID: protoItem.ID, Revision: protoItem.Revision}
	overview, err := crypt.OpenItemPart(vault.Key, vault.ID, protoItem.ID, protoItem.Revision, crypt.Overview, protoItem.Overview)
	if err != nil {
		return nil, fmt.Errorf("item %s: %w", protoItem.ID, err)
	}
	details, err := crypt.OpenItemPart(vault.Key, vault.ID, protoItem.ID, protoItem.Revision, crypt.Details, protoItem.Details)
	if err != nil {
		return nil, fmt.Errorf("item %s: %w", protoItem.ID, err)
	}
	defer clear(details)
	if err := json.Unmarshal(overview, &vaultItem.Overview); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(details, &vaultItem.Details); err != nil {
		return nil, err
	}
	return vaultItem, nil
}

// SaveItem creates or updates an item. vaultItem.Revision is the base revision and is
// replaced by the new one on success. On ErrConflict, current holds the server's version.
func (s *Session) SaveItem(ctx context.Context, vault *Vault, vaultItem *item.Item) (current *item.Item, err error) {
	return s.putItem(ctx, vault, vaultItem, false)
}

func (s *Session) DeleteItem(ctx context.Context, vault *Vault, vaultItem *item.Item) (current *item.Item, err error) {
	return s.putItem(ctx, vault, &item.Item{ID: vaultItem.ID, Revision: vaultItem.Revision}, true)
}

func (s *Session) putItem(ctx context.Context, vault *Vault, vaultItem *item.Item, deleted bool) (*item.Item, error) {
	if vaultItem.ID == "" {
		vaultItem.ID = uuid.NewString()
	}
	revision, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	overview, err := json.Marshal(vaultItem.Overview)
	if err != nil {
		return nil, err
	}
	details, err := json.Marshal(vaultItem.Details)
	if err != nil {
		return nil, err
	}
	defer clear(details)
	encryptedOverview, err := crypt.SealItemPart(vault.Key, vault.ID, vaultItem.ID, revision.String(), crypt.Overview, overview)
	if err != nil {
		return nil, err
	}
	encryptedDetails, err := crypt.SealItemPart(vault.Key, vault.ID, vaultItem.ID, revision.String(), crypt.Details, details)
	if err != nil {
		return nil, err
	}

	_, serverItem, err := s.Client.PutItem(ctx, vault.ID, vaultItem.ID, vaultItem.Revision, &proto.PutItemRequest{
		Revision: revision.String(), Deleted: deleted, Overview: encryptedOverview, Details: encryptedDetails,
	})
	if client.IsStatus(err, http.StatusConflict) {
		if serverItem == nil || serverItem.Deleted {
			return nil, ErrConflict
		}
		current, decryptErr := decryptItem(vault, serverItem)
		if decryptErr != nil {
			return nil, decryptErr
		}
		return current, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	vaultItem.Revision = revision.String()
	return nil, nil
}

// GetVerifiedMember returns a member whose KEM key is signed by their signing key.
// Callers should additionally pin the member's PublicKeys.Sign (trust on first use).
func (s *Session) GetVerifiedMember(ctx context.Context, emailOrID string) (*proto.Member, error) {
	allMember, err := s.Client.ListAllMember(ctx)
	if err != nil {
		return nil, err
	}
	for i := range allMember {
		member := &allMember[i]
		if member.AccountID == emailOrID || strings.EqualFold(member.Email, emailOrID) {
			if err := member.PublicKeys.Verify(member.AccountID); err != nil {
				return nil, fmt.Errorf("member %s: %w", member.Email, err)
			}
			return member, nil
		}
	}
	return nil, fmt.Errorf("member %q: %w", emailOrID, ErrNotFound)
}

func (s *Session) ShareVault(ctx context.Context, vault *Vault, member *proto.Member, role string) error {
	if err := member.PublicKeys.Verify(member.AccountID); err != nil {
		return err
	}
	wrappedKey, err := crypt.WrapVaultKey(member.PublicKeys.KEM, vault.ID, member.AccountID, vault.Key)
	if err != nil {
		return err
	}
	return s.Client.SetAccess(ctx, vault.ID, member.AccountID, &proto.SetAccessRequest{WrappedKey: wrappedKey, Role: role})
}
