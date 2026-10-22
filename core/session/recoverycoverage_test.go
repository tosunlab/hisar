package session

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
)

type r08Transport func(*http.Request) (*http.Response, error)

func (f r08Transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func r08Response(status int, value any) *http.Response {
	body, _ := json.Marshal(value)
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}
}

func TestR08InterruptedPagesPreservePublishedSnapshot(t *testing.T) {
	for _, failure := range []string{"http500", "unreachable", "cancel", "concurrent-local-write"} {
		t.Run(failure, func(t *testing.T) {
			account := createOfflineTestAccount(t)
			c := openTestCache(t)
			ctx := context.Background()
			unlocked, err := Unlock(ctx, account.apiClient, c, account.password, account.secretKey)
			if err != nil {
				t.Fatal(err)
			}
			defer unlocked.Lock()
			vaults, err := unlocked.ListVaults(ctx)
			if err != nil {
				t.Fatal(err)
			}
			healthy := &item.Item{Overview: item.Overview{Title: "Published"}}
			if _, err := unlocked.SaveItem(ctx, &vaults[0], healthy); err != nil {
				t.Fatal(err)
			}
			if _, err := unlocked.ListItems(ctx, &vaults[0]); err != nil {
				t.Fatal(err)
			}
			before, err := c.GetCheckpoint(vaults[0].ID)
			if err != nil || !before.HasCompletedSync {
				t.Fatalf("initial checkpoint: %+v %v", before, err)
			}
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			calls := 0
			localDone := make(chan error, 1)
			account.apiClient.HTTP.Transport = r08Transport(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return r08Response(200, apitypes.ChangesResponse{Items: []apitypes.Item{{ID: healthy.ID, Deleted: true, Seq: 2}}, NextSeq: 2, More: true, KeyGeneration: 1}), nil
				}
				switch failure {
				case "cancel":
					cancel()
					return nil, context.Canceled
				case "concurrent-local-write":
					go func() {
						localDone <- c.ApplyChanges(vaults[0].ID, []apitypes.Item{{ID: "local-write", Revision: "local-revision"}}, 3)
					}()
					select {
					case err := <-localDone:
						t.Fatalf("local writer bypassed unpublished transaction: %v", err)
					case <-time.After(10 * time.Millisecond):
					}
				case "unreachable":
					return r08Response(502, apitypes.Error{Error: "unreachable"}), nil
				}
				return r08Response(500, apitypes.Error{Error: "failed second page"}), nil
			})
			recovered, err := unlocked.ReadItems(ctx, &vaults[0])
			if failure != "unreachable" && failure != "cancel" && err == nil {
				t.Fatal("failed traversal returned success")
			}
			if (failure == "unreachable" || failure == "cancel") && (err != nil || recovered.Provenance.Source != "cached") {
				t.Fatalf("interrupted fallback was not labelled cached: %+v %v", recovered, err)
			}
			if failure == "concurrent-local-write" {
				select {
				case err := <-localDone:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("local writer remained blocked after rollback")
				}
			}
			records, err := c.ListItems(vaults[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, record := range records {
				if record.ID == healthy.ID {
					found = true
				}
			}
			if !found {
				t.Fatalf("failed traversal destroyed prior healthy ciphertext: %+v", records)
			}
			after, err := c.GetCheckpoint(vaults[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if failure == "concurrent-local-write" {
				if after.HasCompletedSync {
					t.Fatal("local write left a false completed checkpoint")
				}
			} else if after != before {
				t.Fatalf("failed traversal published checkpoint: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestR08GenerationMismatchRetryIsBounded(t *testing.T) {
	account := createOfflineTestAccount(t)
	ctx := context.Background()
	unlocked, err := Unlock(ctx, account.apiClient, openTestCache(t), account.password, account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer unlocked.Lock()
	vaults, err := unlocked.ListVaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport
	calls := 0
	account.apiClient.HTTP.Transport = r08Transport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/changes") {
			calls++
			return r08Response(200, apitypes.ChangesResponse{NextSeq: 0, KeyGeneration: 2}), nil
		}
		return transport.RoundTrip(r)
	})
	_, err = unlocked.ReadItems(ctx, &vaults[0])
	if !errors.Is(err, ErrKeyChanged) || calls != 2 {
		t.Fatalf("unbounded or silent mismatch: calls=%d err=%v", calls, err)
	}
}

func TestR08RotatedMetadataCannotStrandOfflineRescueCiphertext(t *testing.T) {
	account := createOfflineTestAccount(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "copy.db")
	c, err := cache.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	unlocked, err := Unlock(ctx, account.apiClient, c, account.password, account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	vaults, err := unlocked.ListVaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unlocked.SaveItem(ctx, &vaults[0], &item.Item{Overview: item.Overview{Title: "Only local rescue"}, Details: item.Details{Password: "synthetic-old-generation"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := unlocked.ListItems(ctx, &vaults[0]); err != nil {
		t.Fatal(err)
	}
	nextKey := crypt.GenerateKey()
	defer clear(nextKey)
	wrapped, err := crypt.WrapVaultKey(unlocked.Keyset.GetKEMPublicKey(), vaults[0].ID, unlocked.AccountID, nextKey)
	if err != nil {
		t.Fatal(err)
	}
	name, err := crypt.SealVaultName(nextKey, vaults[0].ID, []byte("Family"))
	if err != nil {
		t.Fatal(err)
	}
	account.server.mutex.Lock()
	account.server.vaults[0].WrappedKey, account.server.vaults[0].EncName, account.server.vaults[0].KeyGeneration = wrapped, name, 2
	account.server.mutex.Unlock()
	if _, err := unlocked.ReadVaults(ctx); err != nil {
		t.Fatal(err)
	}
	unlocked.Lock()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	account.server.setUnreachable(true)
	c, err = cache.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	unlocked, err = Unlock(ctx, account.apiClient, c, account.password, account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer unlocked.Lock()
	inventory, err := unlocked.ReadVaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := unlocked.ReadItems(ctx, &inventory.Vaults[0])
	if err != nil || len(recovered.Items) != 1 || recovered.Items[0].Details.Password != "synthetic-old-generation" {
		t.Fatalf("verified metadata publication stranded the only completed rescue ciphertext across restart: %+v %v", recovered, err)
	}
}
