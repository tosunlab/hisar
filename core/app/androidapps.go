package app

import (
	"context"
	"errors"
	"time"

	"github.com/tryhisar/hisar/core/item"
)

var errInvalidAndroidApp = errors.New("this is not an Android app's package name and signing certificate")

// ItemAndroidApp is an Android app a login fills into. CertificateSHA256 is its signing certificate's SHA-256 as
// apksigner and assetlinks.json show it: upper-case hex bytes joined by colons.
type ItemAndroidApp struct {
	PackageName       string `json:"packageName"`
	CertificateSHA256 string `json:"certificateSha256"`
}

func convertToItemAndroidApps(apps []item.AndroidApp) []ItemAndroidApp {
	itemApps := make([]ItemAndroidApp, 0, len(apps))
	for _, app := range apps {
		itemApps = append(itemApps, ItemAndroidApp{PackageName: app.PackageName, CertificateSHA256: app.CertificateFingerprint()})
	}
	return itemApps
}

func parseAndroidApp(packageName, certificateSHA256 string) (item.AndroidApp, error) {
	app, err := item.ParseAndroidApp(packageName, certificateSHA256)
	if err != nil {
		return item.AndroidApp{}, errInvalidAndroidApp
	}
	return app, nil
}

// AddAndroidApp links a login to an Android app the user chose to always fill it into, named by its package name and
// the hex SHA-256 of its signing certificate; ListLoginsForAndroidApp then offers the login in that app. Adding a link
// the login already has changes nothing.
func (s *Service) AddAndroidApp(vaultID, itemID, packageName, certificateSHA256 string) (ItemDetail, error) {
	return s.changeAndroidApps(vaultID, itemID, packageName, certificateSHA256, (*item.Item).AddAndroidApp)
}

// RemoveAndroidApp unlinks a login from an Android app; removing a link the login does not have changes nothing.
func (s *Service) RemoveAndroidApp(vaultID, itemID, packageName, certificateSHA256 string) (ItemDetail, error) {
	return s.changeAndroidApps(vaultID, itemID, packageName, certificateSHA256, (*item.Item).RemoveAndroidApp)
}

// changeAndroidApps saves the item when change reports that it changed it.
func (s *Service) changeAndroidApps(vaultID, itemID, packageName, certificateSHA256 string, change func(*item.Item, item.AndroidApp) bool) (ItemDetail, error) {
	app, err := parseAndroidApp(packageName, certificateSHA256)
	if err != nil {
		return ItemDetail{}, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ItemDetail{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return ItemDetail{}, err
	}
	cachedItem, err := s.findItem(vaultID, itemID)
	if err != nil {
		return ItemDetail{}, err
	}
	if !cachedItem.OffersLoginFill() {
		return ItemDetail{}, errors.New("only a login or a server can be filled into an app")
	}
	vaultItem := *cachedItem
	if !change(&vaultItem, app) {
		return s.describeItem(cachedItem), nil
	}
	if err := s.saveSessionItem(ctx, vault, &vaultItem); err != nil {
		return ItemDetail{}, convertError(err)
	}
	s.replaceCachedItem(vaultID, vaultItem)
	return s.describeItem(&vaultItem), nil
}
