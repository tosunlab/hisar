package sharelink

import (
	"context"
	"errors"
	"net/http"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

var (
	ErrGone              = errors.New("this link has expired or was already viewed")
	ErrPassphraseNeeded  = errors.New("this link needs a passphrase")
	errMissingCiphertext = errors.New("the server sent an empty share link")
)

// Fetched holds a link after its one counted view, so a wrong passphrase can be retried without another view.
type Fetched struct {
	Link    *apitypes.PublicShareLink
	LinkKey []byte
}

func (fetched *Fetched) GetPassphraseParams() *crypt.ShareLinkPassphraseParams {
	if fetched.Link.Passphrase == nil {
		return nil
	}
	return &crypt.ShareLinkPassphraseParams{Salt: fetched.Link.Passphrase.Salt, Iterations: fetched.Link.Passphrase.Iterations}
}

func (fetched *Fetched) Open(passphrase []byte) (*Payload, error) {
	params := fetched.GetPassphraseParams()
	if params != nil && len(passphrase) == 0 {
		return nil, ErrPassphraseNeeded
	}
	return Open(fetched.Link.Ciphertext, fetched.LinkKey, passphrase, params)
}

func Fetch(ctx context.Context, httpClient *http.Client, link string) (*Fetched, error) {
	serverURL, id, linkKey, err := ParseURL(link)
	if err != nil {
		return nil, err
	}
	apiClient := apiclient.Create(serverURL)
	if httpClient != nil {
		apiClient.HTTP = httpClient
	}
	publicLink, err := apiClient.OpenPublicShareLink(ctx, id)
	if apiclient.IsStatus(err, http.StatusNotFound) {
		return nil, ErrGone
	}
	if err != nil {
		return nil, err
	}
	if len(publicLink.Ciphertext) == 0 {
		return nil, errMissingCiphertext
	}
	return &Fetched{Link: publicLink, LinkKey: linkKey}, nil
}
