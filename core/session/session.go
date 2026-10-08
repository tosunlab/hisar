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
	Seq(vaultID string) int64
	SetSeq(vaultID string, seq int64)
}

type Session struct {
	Client    *client.Client
	AccountID string
	Email     string
	Name      string
	Role      string
	Keyset    *crypt.Keyset
	Seqs      SeqStore
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

func Signup(ctx context.Context, c *client.Client, invite, email, name string, password []byte, dev proto.DeviceInfo) (*Session, crypt.SecretKey, error) {
	accountID := crypt.NewAccountID()
	sk, err := crypt.NewSecretKey(accountID)
	if err != nil {
		return nil, sk, err
	}
	salt := crypt.NewSalt()
	params := crypt.DefaultArgon2Params
	dk, err := crypt.DeriveKeys(password, salt, params, sk)
	if err != nil {
		return nil, sk, err
	}
	defer dk.Wipe()

	ks, err := crypt.NewKeyset()
	if err != nil {
		return nil, sk, err
	}
	encKS, err := ks.Encrypt(dk.AUK, accountID)
	if err != nil {
		return nil, sk, err
	}

	oc, err := pake.Config().Client()
	if err != nil {
		return nil, sk, err
	}
	regReq, err := oc.RegistrationInit(dk.AuthKey)
	if err != nil {
		return nil, sk, err
	}
	start, err := c.RegisterStart(ctx, &proto.RegisterStartRequest{Invite: invite, Email: email, AccountID: accountID, Request: regReq.Serialize()})
	if err != nil {
		return nil, sk, err
	}
	regResp, err := oc.Deserialize.RegistrationResponse(start.Response)
	if err != nil {
		return nil, sk, err
	}
	record, _, err := oc.RegistrationFinalize(regResp, []byte(accountID), pake.ServerIdentity)
	if err != nil {
		return nil, sk, err
	}
	oc.ClearState()

	sess, err := c.RegisterFinish(ctx, &proto.RegisterFinishRequest{
		Invite:     invite,
		Email:      email,
		Name:       name,
		AccountID:  accountID,
		Record:     record.Serialize(),
		Salt:       salt,
		Argon2:     params,
		EncKeyset:  encKS,
		PublicKeys: ks.PublicKeys(accountID),
		Device:     dev,
	})
	if err != nil {
		return nil, sk, err
	}
	c.Token = sess.Token

	s := &Session{Client: c, AccountID: accountID, Email: email, Name: name, Keyset: ks}
	if _, err := s.CreateVault(ctx, "Personal", proto.VaultPersonal); err != nil {
		return nil, sk, fmt.Errorf("create personal vault: %w", err)
	}
	return s, sk, nil
}

func Login(ctx context.Context, c *client.Client, email string, password []byte, sk crypt.SecretKey, dev proto.DeviceInfo) (*Session, error) {
	pre, err := c.Prelogin(ctx, email)
	if err != nil {
		return nil, err
	}
	dk, err := crypt.DeriveKeys(password, pre.Salt, pre.Argon2, sk)
	if err != nil {
		return nil, err
	}
	defer dk.Wipe()

	oc, err := pake.Config().Client()
	if err != nil {
		return nil, err
	}
	ke1, err := oc.GenerateKE1(dk.AuthKey)
	if err != nil {
		return nil, err
	}
	start, err := c.LoginStart(ctx, &proto.LoginStartRequest{Email: email, KE1: ke1.Serialize()})
	if err != nil {
		return nil, err
	}
	ke2, err := oc.Deserialize.KE2(start.KE2)
	if err != nil {
		return nil, ErrBadCredentials
	}
	ke3, _, _, err := oc.GenerateKE3(ke2, []byte(sk.AccountID), pake.ServerIdentity)
	oc.ClearState()
	if err != nil {
		return nil, ErrBadCredentials
	}
	sess, err := c.LoginFinish(ctx, &proto.LoginFinishRequest{LoginID: start.LoginID, KE3: ke3.Serialize(), Device: dev})
	if client.IsStatus(err, http.StatusUnauthorized) {
		return nil, ErrBadCredentials
	}
	if err != nil {
		return nil, err
	}
	c.Token = sess.Token
	if sess.AccountID != sk.AccountID {
		return nil, ErrBadCredentials
	}
	return open(ctx, c, dk.AUK, sk.AccountID)
}

// Unlock decrypts the keyset using an existing session token.
func Unlock(ctx context.Context, c *client.Client, password []byte, sk crypt.SecretKey) (*Session, error) {
	acct, err := c.Account(ctx)
	if err != nil {
		return nil, err
	}
	if acct.AccountID != sk.AccountID {
		return nil, ErrBadCredentials
	}
	dk, err := crypt.DeriveKeys(password, acct.Salt, acct.Argon2, sk)
	if err != nil {
		return nil, err
	}
	defer dk.Wipe()
	return openAccount(acct, c, dk.AUK)
}

func open(ctx context.Context, c *client.Client, auk []byte, accountID string) (*Session, error) {
	acct, err := c.Account(ctx)
	if err != nil {
		return nil, err
	}
	if acct.AccountID != accountID {
		return nil, ErrBadCredentials
	}
	return openAccount(acct, c, auk)
}

func openAccount(acct *proto.Account, c *client.Client, auk []byte) (*Session, error) {
	ks, err := crypt.DecryptKeyset(auk, acct.AccountID, acct.EncKeyset)
	if err != nil {
		return nil, ErrBadCredentials
	}
	own := ks.PublicKeys(acct.AccountID)
	if !bytes.Equal(own.KEM, acct.PublicKeys.KEM) || !bytes.Equal(own.Sign, acct.PublicKeys.Sign) {
		ks.Wipe()
		return nil, ErrKeyMismatch
	}
	return &Session{Client: c, AccountID: acct.AccountID, Email: acct.Email, Name: acct.Name, Role: acct.Role, Keyset: ks}, nil
}

func (s *Session) CreateVault(ctx context.Context, name, typ string) (*Vault, error) {
	id := uuid.NewString()
	key := crypt.NewKey()
	encName, err := crypt.SealVaultName(key, id, []byte(name))
	if err != nil {
		return nil, err
	}
	wrapped, err := crypt.WrapVaultKey(s.Keyset.PublicKeys(s.AccountID).KEM, id, s.AccountID, key)
	if err != nil {
		return nil, err
	}
	if err := s.Client.CreateVault(ctx, &proto.CreateVaultRequest{ID: id, Type: typ, EncName: encName, WrappedKey: wrapped}); err != nil {
		return nil, err
	}
	return &Vault{ID: id, Type: typ, Name: name, Role: proto.VaultOwner, Key: key}, nil
}

func (s *Session) Vaults(ctx context.Context) ([]Vault, error) {
	vs, err := s.Client.Vaults(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Vault, 0, len(vs))
	for _, v := range vs {
		key, err := s.Keyset.UnwrapVaultKey(v.ID, s.AccountID, v.WrappedKey)
		if err != nil {
			return nil, fmt.Errorf("vault %s: %w", v.ID, err)
		}
		name, err := crypt.OpenVaultName(key, v.ID, v.EncName)
		if err != nil {
			return nil, fmt.Errorf("vault %s: %w", v.ID, err)
		}
		out = append(out, Vault{ID: v.ID, Type: v.Type, Name: string(name), Role: v.Role, Key: key})
	}
	return out, nil
}

func (s *Session) FindVault(ctx context.Context, nameOrID string) (*Vault, error) {
	vs, err := s.Vaults(ctx)
	if err != nil {
		return nil, err
	}
	for i := range vs {
		if vs[i].ID == nameOrID || strings.EqualFold(vs[i].Name, nameOrID) {
			return &vs[i], nil
		}
	}
	return nil, fmt.Errorf("vault %q: %w", nameOrID, ErrNotFound)
}

// Items fetches and decrypts all live items of a vault.
func (s *Session) Items(ctx context.Context, v *Vault) ([]item.Item, error) {
	var since, next int64
	byID := map[string]proto.Item{}
	for {
		resp, err := s.Client.Changes(ctx, v.ID, since, 0)
		if err != nil {
			return nil, err
		}
		for _, it := range resp.Items {
			byID[it.ID] = it
		}
		next = resp.NextSeq
		if !resp.More {
			break
		}
		since = resp.NextSeq
	}
	if s.Seqs != nil {
		if next < s.Seqs.Seq(v.ID) {
			return nil, ErrRollback
		}
		s.Seqs.SetSeq(v.ID, next)
	}

	out := make([]item.Item, 0, len(byID))
	for _, p := range byID {
		if p.Deleted {
			continue
		}
		it, err := decryptItem(v, &p)
		if err != nil {
			return nil, err
		}
		out = append(out, *it)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Overview.Title) < strings.ToLower(out[j].Overview.Title)
	})
	return out, nil
}

func decryptItem(v *Vault, p *proto.Item) (*item.Item, error) {
	it := &item.Item{ID: p.ID, Revision: p.Revision}
	ov, err := crypt.OpenItemPart(v.Key, v.ID, p.ID, p.Revision, crypt.Overview, p.Overview)
	if err != nil {
		return nil, fmt.Errorf("item %s: %w", p.ID, err)
	}
	det, err := crypt.OpenItemPart(v.Key, v.ID, p.ID, p.Revision, crypt.Details, p.Details)
	if err != nil {
		return nil, fmt.Errorf("item %s: %w", p.ID, err)
	}
	defer clear(det)
	if err := json.Unmarshal(ov, &it.Overview); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(det, &it.Details); err != nil {
		return nil, err
	}
	return it, nil
}

// SaveItem creates or updates an item. it.Revision is the base revision and is
// replaced by the new one on success. On ErrConflict, current holds the server's version.
func (s *Session) SaveItem(ctx context.Context, v *Vault, it *item.Item) (current *item.Item, err error) {
	return s.put(ctx, v, it, false)
}

func (s *Session) DeleteItem(ctx context.Context, v *Vault, it *item.Item) (current *item.Item, err error) {
	return s.put(ctx, v, &item.Item{ID: it.ID, Revision: it.Revision}, true)
}

func (s *Session) put(ctx context.Context, v *Vault, it *item.Item, deleted bool) (*item.Item, error) {
	if it.ID == "" {
		it.ID = uuid.NewString()
	}
	rev, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	ov, err := json.Marshal(it.Overview)
	if err != nil {
		return nil, err
	}
	det, err := json.Marshal(it.Details)
	if err != nil {
		return nil, err
	}
	defer clear(det)
	encOv, err := crypt.SealItemPart(v.Key, v.ID, it.ID, rev.String(), crypt.Overview, ov)
	if err != nil {
		return nil, err
	}
	encDet, err := crypt.SealItemPart(v.Key, v.ID, it.ID, rev.String(), crypt.Details, det)
	if err != nil {
		return nil, err
	}

	_, cur, err := s.Client.PutItem(ctx, v.ID, it.ID, it.Revision, &proto.PutItemRequest{
		Revision: rev.String(), Deleted: deleted, Overview: encOv, Details: encDet,
	})
	if client.IsStatus(err, http.StatusConflict) {
		if cur == nil || cur.Deleted {
			return nil, ErrConflict
		}
		current, derr := decryptItem(v, cur)
		if derr != nil {
			return nil, derr
		}
		return current, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	it.Revision = rev.String()
	return nil, nil
}

// VerifiedMember returns a member whose KEM key is signed by their signing key.
// Callers should additionally pin m.PublicKeys.Sign (trust on first use).
func (s *Session) VerifiedMember(ctx context.Context, emailOrID string) (*proto.Member, error) {
	ms, err := s.Client.Members(ctx)
	if err != nil {
		return nil, err
	}
	for i := range ms {
		m := &ms[i]
		if m.AccountID == emailOrID || strings.EqualFold(m.Email, emailOrID) {
			if err := m.PublicKeys.Verify(m.AccountID); err != nil {
				return nil, fmt.Errorf("member %s: %w", m.Email, err)
			}
			return m, nil
		}
	}
	return nil, fmt.Errorf("member %q: %w", emailOrID, ErrNotFound)
}

func (s *Session) Share(ctx context.Context, v *Vault, m *proto.Member, role string) error {
	if err := m.PublicKeys.Verify(m.AccountID); err != nil {
		return err
	}
	wrapped, err := crypt.WrapVaultKey(m.PublicKeys.KEM, v.ID, m.AccountID, v.Key)
	if err != nil {
		return err
	}
	return s.Client.SetAccess(ctx, v.ID, m.AccountID, &proto.SetAccessRequest{WrappedKey: wrapped, Role: role})
}
