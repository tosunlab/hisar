package granthead

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/tryhisar/hisar/core/app"
	"github.com/zalando/go-keyring"
)

// account is the item's account attribute, the one the Mac's keychain item has.
const account = "head"

type store struct {
	service string
}

// New returns the store of the Secret Service item named service (GNOME Keyring, KWallet): the D-Bus API
// core/localstate keeps the Secret Key with. Without a Secret Service every call returns
// app.ErrAgentGrantHeadUnavailable, so no grant outlives a restart.
func New(service string) app.AgentGrantHeadStore {
	return &store{service: service}
}

func (store *store) LoadAgentGrantHead() ([]byte, error) {
	encoded, err := keyring.Get(store.service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, app.ErrNoAgentGrantHead
	}
	if err != nil {
		return nil, describeError(err)
	}
	head, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		// Another program wrote it: as on the Mac, its bytes match no sealed head, so the saved grants are dropped.
		return []byte(encoded), nil
	}
	return head, nil
}

func (store *store) SaveAgentGrantHead(head []byte) error {
	if err := keyring.Set(store.service, account, base64.StdEncoding.EncodeToString(head)); err != nil {
		return describeError(err)
	}
	return nil
}

func (store *store) DeleteAgentGrantHead() error {
	if err := keyring.Delete(store.service, account); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return describeError(err)
	}
	return nil
}

func describeError(err error) error {
	return fmt.Errorf("%w: Secret Service: %v", app.ErrAgentGrantHeadUnavailable, err)
}
