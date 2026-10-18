package clisocket

import (
	"sync"
	"testing"
	"testing/synctest"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

func TestApprovalAnsweredAfterLockIsDenied(t *testing.T) {
	for _, test := range []struct {
		name        string
		request     cliprotocol.Request
		isLauncher  bool
		clientCount int
	}{
		{"command line with waiter", cliprotocol.Request{Type: cliprotocol.TypeListVault}, false, 2},
		{"launcher with waiter", listLauncherItems, true, 2},
		{"command-line share", createShareLinkRequest(), false, 1},
		{"export", cliprotocol.Request{Type: cliprotocol.TypeExport, Export: &cliprotocol.ExportChange{Format: cliprotocol.ExportFormatPlain, Path: "/tmp/hisar-export.json"}}, false, 1},
		{"launcher share", shareLauncherItem(), true, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fixture := createServerFixture(t)
				release := make(chan struct{})
				fixture.approver.release = release
				fixture.launcherApprover.release = release
				requestCount := &fixture.approver.requestCount
				if test.isLauncher {
					requestCount = &fixture.launcherApprover.requestCount
				}
				clients := make([]*testClient, test.clientCount)
				responses := make([]cliprotocol.Response, test.clientCount)
				var waitGroup sync.WaitGroup
				for index := range clients {
					if test.isLauncher {
						clients[index], _ = fixture.connectLauncher(t, raycastApp)
					} else {
						clients[index] = fixture.connect(t)
					}
					waitGroup.Go(func() { responses[index] = clients[index].send(t, test.request) })
				}
				synctest.Wait()
				if count := requestCount.Load(); count != 1 {
					t.Fatalf("%d approval dialogs; want one open before locking", count)
				}
				fixture.approvals.ClearAll()
				close(release)
				waitGroup.Wait()

				for index, response := range responses {
					if getErrorCode(response) != cliprotocol.ErrorDenied || response.Error.Message != ErrDenied.Error() {
						t.Errorf("request %d answered after a lock: %+v", index, response)
					}
					if clients[index].isApproved(t) {
						t.Errorf("request %d remembered an approval answered after a lock", index)
					}
				}
				if len(fixture.handler.types) != 0 {
					t.Errorf("requests reached the handler after a lock: %q", fixture.handler.types)
				}
				if len(fixture.approvals.ListLaunchers()) != 0 {
					t.Error("a launcher approval answered after a lock was remembered")
				}
				clients[0].mustSend(t, test.request)
				if count := requestCount.Load(); count != 2 {
					t.Errorf("%d approvals; the next request must ask again", count)
				}
			})
		})
	}
}
