package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
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
	connection            net.Conn
	socketPath            string
	hello                 cliprotocol.HelloResult
	nextID                int
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
	client := &appClient{connection: connection, socketPath: socketPath}
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
	return client.connection.Close()
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
	client.nextID++
	request.ID = strconv.Itoa(client.nextID)
	client.connection.SetDeadline(time.Now().Add(timeout))
	stopClosing := context.AfterFunc(ctx, func() { client.connection.SetDeadline(time.Now()) })
	defer stopClosing()
	if err := cliprotocol.WriteJSON(client.connection, request); err != nil {
		return client.describeConnectionError(ctx, err)
	}
	var response cliprotocol.Response
	if err := cliprotocol.ReadJSON(client.connection, &response); err != nil {
		return client.describeConnectionError(ctx, err)
	}
	if response.ID != request.ID {
		return errors.New("the Hisar app answered another request")
	}
	if response.Error != nil {
		return response.Error
	}
	if result != nil && len(response.Result) > 0 {
		return json.Unmarshal(response.Result, result)
	}
	return nil
}

func (client *appClient) describeConnectionError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return errors.New("the Hisar app did not answer in time")
	}
	return fmt.Errorf("lost the connection to the Hisar app: %w", err)
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

func listAllAppItem(ctx context.Context, client *appClient, vaultID string) ([]item.Item, error) {
	var itemList cliprotocol.ItemListResult
	if err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeListItem, VaultID: vaultID}, &itemList); err != nil {
		return nil, err
	}
	allItem := make([]item.Item, 0, len(itemList.Items))
	for _, appItem := range itemList.Items {
		allItem = append(allItem, convertFromAppItem(appItem))
	}
	return allItem, nil
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
	allItem, err := listAllAppItem(ctx, client, vault.ID)
	if err != nil {
		return vault, nil, err
	}
	summary, err := findItem(allItem, query)
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
	return &cliprotocol.ItemChange{
		ID: vaultItem.ID, Revision: vaultItem.Revision, Title: vaultItem.Overview.Title, Username: vaultItem.Overview.Username,
		URLs: vaultItem.Overview.URLs, Password: vaultItem.Details.Password, Notes: vaultItem.Details.Notes,
	}
}

func isOneTimeCodeField(fieldName string) bool {
	return strings.EqualFold(fieldName, "code") || strings.EqualFold(fieldName, "one-time code")
}
