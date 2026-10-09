package vault

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/importer"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
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

type ImportDuplicate struct {
	Title    string `json:"title"`
	Username string `json:"username"`
	URL      string `json:"url"`
}

// ImportPreview: an empty FileName means the user cancelled the file dialog.
type ImportPreview struct {
	FileName        string               `json:"fileName"`
	FilePath        string               `json:"filePath"`
	Format          string               `json:"format"`
	FormatName      string               `json:"formatName"`
	VaultID         string               `json:"vaultId"`
	VaultName       string               `json:"vaultName"`
	ItemCount       int                  `json:"itemCount"`
	LoginCount      int                  `json:"loginCount"`
	SecureNoteCount int                  `json:"secureNoteCount"`
	SSHKeyCount     int                  `json:"sshKeyCount"`
	AllSkipped      []ImportSkippedEntry `json:"allSkipped"`
	AllDuplicate    []ImportDuplicate    `json:"allDuplicate"`
}

type ImportProgress struct {
	FileName   string               `json:"fileName"`
	FilePath   string               `json:"filePath"`
	VaultID    string               `json:"vaultId"`
	VaultName  string               `json:"vaultName"`
	SavedCount int                  `json:"savedCount"`
	TotalCount int                  `json:"totalCount"`
	IsDone     bool                 `json:"isDone"`
	AllFailed  []ImportSkippedEntry `json:"allFailed"`
}

// pendingImport holds the parsed export between the preview and saving; locking drops it.
type pendingImport struct {
	filePath      string
	format        string
	allItem       []item.Item
	allSkipped    []importer.SkippedEntry
	isStarted     bool
	vaultID       string
	allQueuedItem []item.Item
	savedCount    int
	allFailed     []ImportSkippedEntry
}

func (s *Service) ListAllImportFormat() []ImportFormat {
	allFormat := make([]ImportFormat, 0, len(importer.AllFormat))
	for _, format := range importer.AllFormat {
		allFormat = append(allFormat, ImportFormat{ID: format.ID, Name: format.Name})
	}
	return allFormat
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
	s.pendingImport = &pendingImport{filePath: path, format: result.Format, allItem: result.AllItem, allSkipped: result.AllSkipped}
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
	if vault.Role == proto.VaultReader {
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
	if vault.Role == proto.VaultReader {
		return nil, nil, errReadOnlyVault
	}
	allExisting, err := s.unlockedSession.ListAllItem(ctx, vault)
	if err != nil {
		return nil, nil, convertError(err)
	}
	s.allItemByVaultID[vault.ID] = allExisting
	return vault, allExisting, nil
}

func (s *Service) buildImportPreview(vaultID string) (ImportPreview, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, allExisting, err := s.loadEditableVaultItem(ctx, vaultID)
	if err != nil {
		return ImportPreview{}, err
	}
	pending := s.pendingImport
	summary := importer.Summarize(pending.allItem)
	preview := ImportPreview{
		FileName:        filepath.Base(pending.filePath),
		FilePath:        pending.filePath,
		Format:          pending.format,
		FormatName:      importer.GetFormatName(pending.format),
		VaultID:         vault.ID,
		VaultName:       vault.Name,
		ItemCount:       len(pending.allItem),
		LoginCount:      summary.LoginCount,
		SecureNoteCount: summary.SecureNoteCount,
		SSHKeyCount:     summary.SSHKeyCount,
		AllSkipped:      []ImportSkippedEntry{},
		AllDuplicate:    []ImportDuplicate{},
	}
	for _, skipped := range pending.allSkipped {
		preview.AllSkipped = append(preview.AllSkipped, ImportSkippedEntry{Title: skipped.Title, Reason: skipped.Reason})
	}
	for _, index := range importer.FindAllDuplicateIndex(pending.allItem, allExisting) {
		duplicate := pending.allItem[index]
		firstURL := ""
		if len(duplicate.Overview.URLs) > 0 {
			firstURL = duplicate.Overview.URLs[0]
		}
		preview.AllDuplicate = append(preview.AllDuplicate, ImportDuplicate{Title: duplicate.Overview.Title, Username: duplicate.Overview.Username, URL: firstURL})
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
	vault, allExisting, err := s.loadEditableVaultItem(ctx, vaultID)
	if err != nil {
		return ImportProgress{}, err
	}
	allQueuedItem := slices.Clone(pending.allItem)
	if isSkippingDuplicate {
		allDuplicateIndex := importer.FindAllDuplicateIndex(pending.allItem, allExisting)
		allQueuedItem = allQueuedItem[:0]
		for index, vaultItem := range pending.allItem {
			if !slices.Contains(allDuplicateIndex, index) {
				allQueuedItem = append(allQueuedItem, vaultItem)
			}
		}
	}
	pending.isStarted, pending.vaultID, pending.allQueuedItem, pending.allItem = true, vault.ID, allQueuedItem, nil
	pending.allFailed = []ImportSkippedEntry{}
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
	for batchCount := 0; batchCount < importBatchSize && len(pending.allQueuedItem) > 0; batchCount++ {
		vaultItem := pending.allQueuedItem[0]
		_, err := s.unlockedSession.SaveItem(ctx, vault, &vaultItem)
		if isImportStoppingError(err) {
			return ImportProgress{}, convertError(err)
		}
		pending.allQueuedItem = pending.allQueuedItem[1:]
		if err != nil {
			pending.allFailed = append(pending.allFailed, ImportSkippedEntry{Title: vaultItem.Overview.Title, Reason: convertError(err).Error()})
			continue
		}
		pending.savedCount++
		s.replaceCachedItem(vault.ID, vaultItem)
	}
	return s.buildImportProgress(), nil
}

// isImportStoppingError: these failures would repeat for every item, so the import pauses and can be continued.
func isImportStoppingError(err error) bool {
	return err != nil && (client.IsUnreachable(err) || client.IsStatus(err, http.StatusUnauthorized) ||
		client.IsStatus(err, http.StatusForbidden) || client.IsStatus(err, http.StatusTooManyRequests) || errors.Is(err, context.DeadlineExceeded))
}

func (s *Service) buildImportProgress() ImportProgress {
	pending := s.pendingImport
	progress := ImportProgress{
		FileName:   filepath.Base(pending.filePath),
		FilePath:   pending.filePath,
		VaultID:    pending.vaultID,
		SavedCount: pending.savedCount,
		TotalCount: pending.savedCount + len(pending.allFailed) + len(pending.allQueuedItem),
		IsDone:     len(pending.allQueuedItem) == 0,
		AllFailed:  slices.Clone(pending.allFailed),
	}
	if vault, found := s.allVaultByID[pending.vaultID]; found {
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
	if pending == nil || !pending.isStarted || len(pending.allQueuedItem) > 0 {
		return ErrNoPendingImport
	}
	if err := os.Remove(pending.filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not delete %s; delete it in Finder", filepath.Base(pending.filePath))
	}
	s.pendingImport = nil
	return nil
}
