package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
)

const (
	standaloneEnvironment = "HISAR_STANDALONE"
	appDialTimeout        = 2 * time.Second
	appHelloTimeout       = 5 * time.Second
	// The approval window waits 60 s for an answer; unlocking with a typed password can take longer.
	appRequestTimeout = 5 * time.Minute
)

type appClient struct {
	protocolClient        *cliprotocol.Client
	socketPath            string
	hello                 cliprotocol.HelloResult
	isApprovalNoticeShown bool
}

// commandMode is how this run reaches the vaults: through the app (client set) or standalone, and why.
type commandMode struct {
	client          *appClient
	reason          string
	socketPath      string
	isAppNotRunning bool
}

var selectedMode commandMode

func isStandaloneRequested() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(standaloneEnvironment)))
	return value == "1" || value == "true" || value == "yes"
}

// chooseMode uses the app when it answers hello, starting it when nothing listens; the switch being off falls back.
func chooseMode(isStandaloneForced bool) commandMode {
	return chooseModeWithLaunch(isStandaloneForced, true)
}

func chooseModeWithLaunch(isStandaloneForced, isLaunchAllowed bool) commandMode {
	if isStandaloneForced {
		return commandMode{reason: "--standalone was given"}
	}
	if isStandaloneRequested() {
		return commandMode{reason: standaloneEnvironment + " is set"}
	}
	socketPath, err := cliprotocol.GetSocketPath()
	if err != nil {
		return commandMode{reason: "the Hisar app's socket could not be found: " + err.Error()}
	}
	return connectToApp(socketPath, isLaunchAllowed)
}

func dialApp(socketPath string) (*appClient, error) {
	connection, err := net.DialTimeout("unix", socketPath, appDialTimeout)
	if err != nil {
		return nil, &appUnreachableError{socketPath: socketPath}
	}
	client := &appClient{protocolClient: cliprotocol.NewClient(connection), socketPath: socketPath}
	if err := client.callWithTimeout(context.Background(), cliprotocol.Request{Type: cliprotocol.TypeHello}, &client.hello, appHelloTimeout); err != nil {
		connection.Close()
		var responseError *cliprotocol.ResponseError
		if errors.As(err, &responseError) && responseError.Code == cliprotocol.ErrorIntegrationDisabled {
			return nil, errors.New(`the Hisar app has "Let the command-line tool use Hisar" turned off`)
		}
		return nil, errors.New("the Hisar app did not answer on " + socketPath)
	}
	if client.hello.ProtocolVersion != cliprotocol.ProtocolVersion {
		connection.Close()
		return nil, fmt.Errorf("the Hisar app speaks protocol version %d, this hisar %d; update both", client.hello.ProtocolVersion, cliprotocol.ProtocolVersion)
	}
	return client, nil
}

func (client *appClient) Close() error {
	return client.protocolClient.Close()
}

func (client *appClient) showApprovalNotice() {
	if client.isApprovalNoticeShown || client.hello.IsApproved {
		return
	}
	client.isApprovalNoticeShown = true
	if client.hello.State == cliprotocol.StateLocked {
		fmt.Fprintln(os.Stderr, "Unlock Hisar in its approval window to let hisar in this terminal use it…")
		return
	}
	fmt.Fprintln(os.Stderr, "Approve hisar in this terminal in Hisar's approval window…")
}

func (client *appClient) call(ctx context.Context, request cliprotocol.Request, result any) error {
	client.showApprovalNotice()
	return client.callWithTimeout(ctx, request, result, appRequestTimeout)
}

func (client *appClient) callWithTimeout(ctx context.Context, request cliprotocol.Request, result any, timeout time.Duration) error {
	request.ClientPlatform, request.ClientVersion = getPlatform(), getClientVersion()
	return client.protocolClient.Call(ctx, request, result, timeout)
}

func findAppVault(ctx context.Context, client *appClient, nameOrID string) (cliprotocol.Vault, error) {
	var vaultList cliprotocol.VaultListResult
	if err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeListVault}, &vaultList); err != nil {
		return cliprotocol.Vault{}, err
	}
	for _, vault := range vaultList.Vaults {
		if vault.ID == nameOrID || strings.EqualFold(vault.Name, nameOrID) {
			return vault, nil
		}
	}
	return cliprotocol.Vault{}, fmt.Errorf("vault %q: not found", nameOrID)
}

func listAppItems(ctx context.Context, client *appClient, vaultID string) ([]item.Item, error) {
	var itemList cliprotocol.ItemListResult
	if err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeListItem, VaultID: vaultID}, &itemList); err != nil {
		return nil, err
	}
	items := make([]item.Item, 0, len(itemList.Items))
	for _, appItem := range itemList.Items {
		items = append(items, convertFromAppItem(appItem))
	}
	return items, nil
}

func getAppItem(ctx context.Context, client *appClient, vaultID, itemID string) (*item.Item, error) {
	var itemResult cliprotocol.ItemResult
	if err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeGetItem, VaultID: vaultID, ItemID: itemID}, &itemResult); err != nil {
		return nil, err
	}
	vaultItem := convertFromAppItem(itemResult.Item)
	return &vaultItem, nil
}

// findAppItem finds the item among the vault's summaries, then fetches it with its secrets.
func findAppItem(ctx context.Context, client *appClient, vaultName, query string) (cliprotocol.Vault, *item.Item, error) {
	vault, err := findAppVault(ctx, client, vaultName)
	if err != nil {
		return vault, nil, err
	}
	items, err := listAppItems(ctx, client, vault.ID)
	if err != nil {
		return vault, nil, err
	}
	summary, err := findItem(items, query)
	if err != nil {
		return vault, nil, err
	}
	vaultItem, err := getAppItem(ctx, client, vault.ID, summary.ID)
	return vault, vaultItem, err
}

func convertFromAppItem(appItem cliprotocol.Item) item.Item {
	vaultItem := item.Item{
		ID:       appItem.ID,
		Revision: appItem.Revision,
		Overview: item.Overview{Category: appItem.Category, Title: appItem.Title, Username: appItem.Username, URLs: appItem.URLs, PublicKey: appItem.PublicKey, Fingerprint: appItem.Fingerprint},
		Details:  item.Details{Password: appItem.Password, Notes: appItem.Notes},
	}
	for _, field := range appItem.Fields {
		vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: field.Name, Value: field.Value, Concealed: field.Concealed})
	}
	for _, passkey := range appItem.Passkeys {
		vaultItem.Details.Passkeys = append(vaultItem.Details.Passkeys, item.Passkey{RelyingPartyID: passkey.RelyingPartyID, UserName: passkey.UserName})
	}
	return vaultItem
}

func convertToItemChange(vaultItem *item.Item) *cliprotocol.ItemChange {
	change := &cliprotocol.ItemChange{
		ID: vaultItem.ID, Revision: vaultItem.Revision, Title: vaultItem.Overview.Title, Username: vaultItem.Overview.Username,
		URLs: vaultItem.Overview.URLs, Password: vaultItem.Details.Password, Notes: vaultItem.Details.Notes, Category: vaultItem.Overview.Category,
	}
	// A card's, identity's, API credential's or address's values are its named fields, so the fields travel with it.
	if vaultItem.IsCategory(item.CategoryCreditCard) || vaultItem.IsCategory(item.CategoryIdentity) || vaultItem.IsCategory(item.CategoryAPICredential) ||
		vaultItem.IsCategory(item.CategoryAddress) {
		fields := []cliprotocol.Field{}
		for _, field := range vaultItem.Details.Fields {
			fields = append(fields, cliprotocol.Field{Name: field.Name, Value: field.Value, Concealed: field.Concealed})
		}
		change.Fields = &fields
	}
	return change
}

func isOneTimeCodeField(fieldName string) bool {
	return strings.EqualFold(fieldName, "code") || strings.EqualFold(fieldName, "one-time code")
}
