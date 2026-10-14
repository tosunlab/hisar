package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
)

// checkRemoteChangeFlow saves an item from a second device and expects the first to sync it through the event stream.
func checkRemoteChangeFlow(t *testing.T, service *Service, serverURL, email, secretKey, password, personalID string) {
	t.Helper()
	remoteChangeChannel := make(chan struct{}, 4)
	service.mutex.Lock()
	service.config.OnRemoteChange = func() { remoteChangeChannel <- struct{}{} }
	service.mutex.Unlock()
	defer func() {
		service.mutex.Lock()
		service.config.OnRemoteChange = nil
		service.mutex.Unlock()
	}()
	secondDevice, _, _ := createTestService(t, filepath.Join(t.TempDir(), "second-device.json"), createMemorySecretKeyStore())
	secondDevice.config.IsEventStreamDisabled = true
	if _, err := secondDevice.SignIn(serverURL, email, secretKey, password); err != nil {
		t.Fatal(err)
	}
	defer secondDevice.SignOut()
	time.Sleep(500 * time.Millisecond)
	saved, err := secondDevice.SaveItem(personalID, ItemDetail{Title: "From the second device", Username: "pushed", Fields: []ItemField{}})
	if err != nil {
		t.Fatal(err)
	}
	// Late events from this device's own earlier saves can arrive first, so wait for the one that brings the item.
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-remoteChangeChannel:
		case <-deadline:
			detail, err := service.GetItem(personalID, saved.ID)
			t.Fatalf("the first device did not sync the change: %+v %v", detail, err)
		}
		if detail, err := service.GetItem(personalID, saved.ID); err == nil && detail.Username == "pushed" {
			break
		}
	}
	if err := service.DeleteItem(personalID, saved.ID); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteChangeSyncsAndNotifies(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	remoteChangeChannel := make(chan struct{}, 4)
	service.mutex.Lock()
	service.config.OnRemoteChange = func() { remoteChangeChannel <- struct{}{} }
	unlockedSession := service.unlockedSession
	service.mutex.Unlock()
	testService.apiServer.mutex.Lock()
	testService.apiServer.eventChannel = make(chan string, 4)
	eventChannel := testService.apiServer.eventChannel
	testService.apiServer.mutex.Unlock()

	if _, err := service.ListAllVault(); err != nil {
		t.Fatal(err)
	}
	family, err := service.findVault(context.Background(), testService.familyID)
	if err != nil {
		t.Fatal(err)
	}
	// Another device saves an item; this service's cache does not know it yet.
	if _, err := unlockedSession.SaveItem(context.Background(), family, &item.Item{Overview: item.Overview{Title: "Saved elsewhere"}}); err != nil {
		t.Fatal(err)
	}
	service.mutex.Lock()
	service.allItemByVaultID[testService.familyID] = []item.Item{}
	service.mutex.Unlock()

	deadline := time.After(10 * time.Second)
	for isSent := false; !isSent; {
		select {
		case eventChannel <- apitypes.EventVaultChanged:
			isSent = true
		case <-deadline:
			t.Fatal("the service never opened the event stream")
		}
	}
	select {
	case <-remoteChangeChannel:
	case <-deadline:
		t.Fatal("no remote change notification")
	}
	service.mutex.Lock()
	allCachedItem := service.allItemByVaultID[testService.familyID]
	service.mutex.Unlock()
	if len(allCachedItem) != 1 || allCachedItem[0].Overview.Title != "Saved elsewhere" {
		t.Errorf("cache after the event = %+v", allCachedItem)
	}

	service.Lock()
	select {
	case eventChannel <- apitypes.EventVaultChanged:
		select {
		case <-remoteChangeChannel:
			t.Error("a locked service synced")
		case <-time.After(300 * time.Millisecond):
		}
	case <-time.After(300 * time.Millisecond):
	}
}

func TestReconnectEventStreamOpensANewStreamNow(t *testing.T) {
	testService := createUnlockedTestService(t)
	apiServer := testService.apiServer
	countStream := func() int {
		apiServer.mutex.Lock()
		defer apiServer.mutex.Unlock()
		return apiServer.eventStreamCount
	}
	waitForStreamCount := func(want int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for countStream() < want {
			if time.Now().After(deadline) {
				t.Fatalf("%d event streams, want %d", countStream(), want)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	apiServer.mutex.Lock()
	apiServer.eventChannel = make(chan string)
	apiServer.mutex.Unlock()
	ReconnectEventStream(testService.service)
	waitForStreamCount(1)
	connectedCount := countStream()
	ReconnectEventStream(testService.service)
	waitForStreamCount(connectedCount + 1)
	ReconnectEventStream(testService.service)
	waitForStreamCount(connectedCount + 2)

	testService.service.Lock()
	ReconnectEventStream(testService.service)
	lockedCount := countStream()
	time.Sleep(300 * time.Millisecond)
	if countStream() != lockedCount {
		t.Error("a locked service opened an event stream")
	}
}
