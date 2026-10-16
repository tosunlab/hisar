package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/importer"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

const (
	maxImportFileSize = 100 << 20
	importBatchSize   = 10
)

var (
	ErrNoPendingImport = errors.New("choose the file to import again")
	errReadOnlyVault   = errors.New("you can only read this vault; choose a vault you can edit")
)

type ImportFormat struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ImportSkippedEntry struct {
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// ImportDuplicate: Detail is what matched besides the title, such as a login's username and website or an API
// credential's key ID; see importer.DescribeDuplicate.
type ImportDuplicate struct {
	Title    string `json:"title"`
	Username string `json:"username"`
	URL      string `json:"url"`
	Detail   string `json:"detail"`
}

// ImportPreview: an empty FileName means the user cancelled the file dialog.
type ImportPreview struct {
	FileName        string `json:"fileName"`
	FilePath        string `json:"filePath"`
	Format          string `json:"format"`
	FormatName      string `json:"formatName"`
	VaultID         string `json:"vaultId"`
	VaultName       string `json:"vaultName"`
	ItemCount       int    `json:"itemCount"`
	LoginCount      int    `json:"loginCount"`
	SecureNoteCount int    `json:"secureNoteCount"`
	SSHKeyCount     int    `json:"sshKeyCount"`
	CreditCardCount int    `json:"creditCardCount"`
	IdentityCount   int    `json:"identityCount"`
	// APICredentialCount counts the api-credential items among ItemCount.
	APICredentialCount int `json:"apiCredentialCount"`
	// AddressCount counts the address items among ItemCount.
	AddressCount int                  `json:"addressCount"`
	Skipped      []ImportSkippedEntry `json:"allSkipped"`
	Duplicates   []ImportDuplicate    `json:"allDuplicate"`
}

type ImportProgress struct {
	FileName   string               `json:"fileName"`
	FilePath   string               `json:"filePath"`
	VaultID    string               `json:"vaultId"`
	VaultName  string               `json:"vaultName"`
	SavedCount int                  `json:"savedCount"`
	TotalCount int                  `json:"totalCount"`
	IsDone     bool                 `json:"isDone"`
	Failed     []ImportSkippedEntry `json:"allFailed"`
}

// pendingImport holds the parsed export between the preview and saving; locking drops it.
type pendingImport struct {
	filePath    string
	format      string
	items       []item.Item
	skipped     []importer.SkippedEntry
	isStarted   bool
	vaultID     string
	queuedItems []item.Item
	savedCount  int
	failed      []ImportSkippedEntry
	// favoriteItemIDs are saved items the other password manager starred; they become favorites when the import ends.
	favoriteItemIDs []string
}

func (s *Service) ListImportFormats() []ImportFormat {
	formats := make([]ImportFormat, 0, len(importer.Formats()))
	for _, format := range importer.Formats() {
		formats = append(formats, ImportFormat{ID: format.ID, Name: format.Name})
	}
	return formats
}

// ChooseImportFile asks for an export file and reads it; format "" detects it from the file.
func (s *Service) ChooseImportFile(vaultID, format string) (ImportPreview, error) {
	if !importer.IsKnownFormat(format) {
		return ImportPreview{}, errors.New("choose one of the supported import formats")
	}
	if err := s.checkEditableVault(vaultID); err != nil {
		return ImportPreview{}, err
	}
	if s.config.FileOpener == nil {
		return ImportPreview{}, errors.New("opening files is not available")
	}
	path, err := s.config.FileOpener.ChooseOpenPath()
	if err != nil || path == "" {
		return ImportPreview{}, err
	}
	content, err := readImportFile(path)
	if err != nil {
		return ImportPreview{}, err
	}
	result, err := importer.Parse(filepath.Base(path), content, format)
	clear(content)
	if err != nil {
		return ImportPreview{}, err
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ImportPreview{}, err
	}
	s.pendingImport = &pendingImport{filePath: path, format: result.Format, items: result.Items, skipped: result.Skipped}
	return s.buildImportPreview(vaultID)
}

func readImportFile(path string) ([]byte, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("could not open %s", filepath.Base(path))
	}
	if fileInfo.Size() > maxImportFileSize {
		return nil, errors.New("the file is larger than 100 MB; export without attachments and try again")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read %s", filepath.Base(path))
	}
	return content, nil
}

func (s *Service) checkEditableVault(vaultID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return err
	}
	if vault.Role == apitypes.VaultReader {
		return errReadOnlyVault
	}
	return nil
}

// PreviewImport shows the chosen file again for another vault, with that vault's duplicates.
func (s *Service) PreviewImport(vaultID string) (ImportPreview, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ImportPreview{}, err
	}
	if s.pendingImport == nil || s.pendingImport.isStarted {
		return ImportPreview{}, ErrNoPendingImport
	}
	return s.buildImportPreview(vaultID)
}

func (s *Service) loadEditableVaultItem(ctx context.Context, vaultID string) (*session.Vault, []item.Item, error) {
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return nil, nil, err
	}
	if vault.Role == apitypes.VaultReader {
		return nil, nil, errReadOnlyVault
	}
	existingItems, err := s.unlockedSession.ListItems(ctx, vault)
	if err != nil {
		return nil, nil, convertError(err)
	}
	s.itemsByVaultID[vault.ID] = existingItems
	return vault, existingItems, nil
}

func (s *Service) buildImportPreview(vaultID string) (ImportPreview, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, existingItems, err := s.loadEditableVaultItem(ctx, vaultID)
	if err != nil {
		return ImportPreview{}, err
	}
	pending := s.pendingImport
	summary := importer.Summarize(pending.items)
	preview := ImportPreview{
		FileName:           filepath.Base(pending.filePath),
		FilePath:           pending.filePath,
		Format:             pending.format,
		FormatName:         importer.GetFormatName(pending.format),
		VaultID:            vault.ID,
		VaultName:          vault.Name,
		ItemCount:          len(pending.items),
		LoginCount:         summary.LoginCount,
		SecureNoteCount:    summary.SecureNoteCount,
		SSHKeyCount:        summary.SSHKeyCount,
		CreditCardCount:    summary.CreditCardCount,
		IdentityCount:      summary.IdentityCount,
		APICredentialCount: summary.APICredentialCount,
		AddressCount:       summary.AddressCount,
		Skipped:            []ImportSkippedEntry{},
		Duplicates:         []ImportDuplicate{},
	}
	for _, skipped := range pending.skipped {
		preview.Skipped = append(preview.Skipped, ImportSkippedEntry{Title: skipped.Title, Reason: skipped.Reason})
	}
	for _, index := range importer.FindDuplicateIndexes(pending.items, existingItems) {
		duplicate := pending.items[index]
		firstURL := ""
		if len(duplicate.Overview.URLs) > 0 {
			firstURL = duplicate.Overview.URLs[0]
		}
		preview.Duplicates = append(preview.Duplicates, ImportDuplicate{
			Title: duplicate.Overview.Title, Username: duplicate.Overview.Username, URL: firstURL, Detail: importer.DescribeDuplicate(duplicate),
		})
	}
	return preview, nil
}

// StartImport queues the previewed items for vaultID; ContinueImport then saves them in batches.
func (s *Service) StartImport(vaultID string, isSkippingDuplicate bool) (ImportProgress, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ImportProgress{}, err
	}
	pending := s.pendingImport
	if pending == nil || pending.isStarted {
		return ImportProgress{}, ErrNoPendingImport
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, existingItems, err := s.loadEditableVaultItem(ctx, vaultID)
	if err != nil {
		return ImportProgress{}, err
	}
	queuedItems := slices.Clone(pending.items)
	if isSkippingDuplicate {
		duplicateIndexes := importer.FindDuplicateIndexes(pending.items, existingItems)
		queuedItems = queuedItems[:0]
		for index, vaultItem := range pending.items {
			if !slices.Contains(duplicateIndexes, index) {
				queuedItems = append(queuedItems, vaultItem)
			}
		}
	}
	pending.isStarted, pending.vaultID, pending.queuedItems, pending.items = true, vault.ID, queuedItems, nil
	pending.failed = []ImportSkippedEntry{}
	return s.buildImportProgress(), nil
}

func (s *Service) ContinueImport() (ImportProgress, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ImportProgress{}, err
	}
	pending := s.pendingImport
	if pending == nil || !pending.isStarted {
		return ImportProgress{}, ErrNoPendingImport
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, pending.vaultID)
	if err != nil {
		return ImportProgress{}, err
	}
	for batchCount := 0; batchCount < importBatchSize && len(pending.queuedItems) > 0; batchCount++ {
		vaultItem := pending.queuedItems[0]
		_, err := s.unlockedSession.SaveItem(ctx, vault, &vaultItem)
		if isImportStoppingError(err) {
			return ImportProgress{}, convertError(err)
		}
		pending.queuedItems = pending.queuedItems[1:]
		if err != nil {
			pending.failed = append(pending.failed, ImportSkippedEntry{Title: vaultItem.Overview.Title, Reason: convertError(err).Error()})
			continue
		}
		pending.savedCount++
		s.replaceCachedItem(vault.ID, vaultItem)
		if vaultItem.IsFavorite {
			pending.favoriteItemIDs = append(pending.favoriteItemIDs, vaultItem.ID)
		}
	}
	if len(pending.queuedItems) == 0 {
		s.addImportedFavorites(ctx, pending.favoriteItemIDs)
		pending.favoriteItemIDs = nil
	}
	return s.buildImportProgress(), nil
}

// isImportStoppingError: these failures would repeat for every item, so the import pauses and can be continued.
func isImportStoppingError(err error) bool {
	return err != nil && (apiclient.IsUnreachable(err) || apiclient.IsStatus(err, http.StatusUnauthorized) ||
		apiclient.IsStatus(err, http.StatusForbidden) || apiclient.IsStatus(err, http.StatusTooManyRequests) || errors.Is(err, context.DeadlineExceeded))
}

func (s *Service) buildImportProgress() ImportProgress {
	pending := s.pendingImport
	progress := ImportProgress{
		FileName:   filepath.Base(pending.filePath),
		FilePath:   pending.filePath,
		VaultID:    pending.vaultID,
		SavedCount: pending.savedCount,
		TotalCount: pending.savedCount + len(pending.failed) + len(pending.queuedItems),
		IsDone:     len(pending.queuedItems) == 0,
		Failed:     slices.Clone(pending.failed),
	}
	if vault, found := s.vaultsByID[pending.vaultID]; found {
		progress.VaultName = vault.Name
	}
	return progress
}

func (s *Service) CancelImport() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.pendingImport = nil
}

// DeleteImportFile removes the export file after a finished import, since it holds every password in plain text.
func (s *Service) DeleteImportFile() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	pending := s.pendingImport
	if pending == nil || !pending.isStarted || len(pending.queuedItems) > 0 {
		return ErrNoPendingImport
	}
	if err := os.Remove(pending.filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not delete %s; delete it in Finder", filepath.Base(pending.filePath))
	}
	s.pendingImport = nil
	return nil
}
