package main

import (
	"context"
	"log"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// systemNotifications shows notifications in the system's notification center. It is not a bound service: it starts
// with the first notification, which on macOS asks for permission, and a notification it cannot show is only logged.
type systemNotifications struct {
	service     *notifications.NotificationService
	startOnce   sync.Once
	startError  error
	mutex       sync.Mutex
	onClickByID map[string]func()
}

func newSystemNotifications() *systemNotifications {
	systemNotifications := &systemNotifications{service: notifications.New(), onClickByID: map[string]func(){}}
	systemNotifications.service.OnNotificationResponse(systemNotifications.respond)
	return systemNotifications
}

// show shows a notification with an ID that replaces an earlier one with the same ID; clicking it runs onClick. It
// returns at once.
func (systemNotifications *systemNotifications) show(id, title, text string, onClick func()) {
	go func() {
		if err := systemNotifications.start(); err != nil {
			log.Printf("notification %q: %v", title, err)
			return
		}
		if isAuthorized, err := systemNotifications.service.RequestNotificationAuthorization(); err != nil || !isAuthorized {
			log.Printf("notification %q: notifications are not allowed: %v", title, err)
			return
		}
		systemNotifications.mutex.Lock()
		systemNotifications.onClickByID[id] = onClick
		systemNotifications.mutex.Unlock()
		if err := systemNotifications.service.SendNotification(notifications.NotificationOptions{ID: id, Title: title, Body: text}); err != nil {
			log.Printf("notification %q: %v", title, err)
		}
	}()
}

func (systemNotifications *systemNotifications) start() error {
	systemNotifications.startOnce.Do(func() {
		systemNotifications.startError = systemNotifications.service.ServiceStartup(context.Background(), application.ServiceOptions{})
	})
	return systemNotifications.startError
}

func (systemNotifications *systemNotifications) respond(result notifications.NotificationResult) {
	if result.Error != nil || result.Response.ActionIdentifier != notifications.DefaultActionIdentifier {
		return
	}
	systemNotifications.mutex.Lock()
	onClick := systemNotifications.onClickByID[result.Response.ID]
	delete(systemNotifications.onClickByID, result.Response.ID)
	systemNotifications.mutex.Unlock()
	if onClick != nil {
		onClick()
	}
}
