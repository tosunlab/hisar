package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/export"
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
// credential's key ID; see importer.DescribeDuplicate. PasskeyCount is how many of its passkeys the existing item lacks;
// skipping duplicates adds them to it.
type ImportDuplicate struct {
	Title        string `json:"title"`
	Username     string `json:"username"`
	URL          string `json:"url"`
	Detail       string `json:"detail"`
	PasskeyCount int    `json:"passkeyCount"`
}

// ImportPreview: an empty FileName means the user cancelled the file dialog. A Credential Exchange import has no file:
// FileName names the app the items came from and FilePath is empty. An encrypted Hisar export first comes back with only
// the file and IsPassphraseRequired; OpenImportFile with its passphrase returns the full preview. Notice describes a
// Hisar export that is a cached or partial copy, and IsSourceKept says the file stays: Hisar never deletes an encrypted
// export, a backup, nor a copy that may not hold everything. ExportedFrom and ExportedAt name the account and the time
// of a Hisar export.
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
	AddressCount int `json:"addressCount"`
	// SoftwareLicenseCount counts the software-license items among ItemCount.
	SoftwareLicenseCount int `json:"softwareLicenseCount"`
	// ServerCount counts the server items among ItemCount.
	ServerCount int `json:"serverCount"`
	// DatabaseCount counts the database items among ItemCount.
	DatabaseCount int `json:"databaseCount"`
	// PasskeyCount counts the passkeys the items hold.
	PasskeyCount int                  `json:"passkeyCount"`
	Skipped      []ImportSkippedEntry `json:"allSkipped"`
	Duplicates   []ImportDuplicate    `json:"allDuplicate"`
	Groups       []ImportSourceGroup  `json:"groups"`
	// IsPassphraseRequired: the file is an encrypted Hisar export; OpenImportFile opens it.
	IsPassphraseRequired bool   `json:"isPassphraseRequired"`
	Notice               string `json:"notice"`
	IsSourceKept         bool   `json:"isSourceKept"`
	ExportedFrom         string `json:"exportedFrom"`
	ExportedAt           string `json:"exportedAt"`
}

type ImportProgress struct {
	FileName        string               `json:"fileName"`
	FilePath        string               `json:"filePath"`
	VaultID         string               `json:"vaultId"`
	VaultName       string               `json:"vaultName"`
	SavedCount      int                  `json:"savedCount"`
	TotalCount      int                  `json:"totalCount"`
	LeftBehindCount int                  `json:"leftBehindCount"`
	IsDone          bool                 `json:"isDone"`
	Failed          []ImportSkippedEntry `json:"allFailed"`
	// IsSourceKept: the file is not offered for deletion, see ImportPreview.
	IsSourceKept bool `json:"isSourceKept"`
}

// pendingImport holds the parsed export between the preview and saving; locking drops it. A Credential Exchange import
// has no filePath; sourceName names the app it came from.
type pendingImport struct {
	filePath       string
	sourceName     string
	format         string
	items          []item.Item
	skipped        []importer.SkippedEntry
	groups         []importDestination
	queuedVaultIDs []string
	scopes         map[string]string
	isStarted      bool
	vaultID        string
	queuedItems    []item.Item
	savedCount     int
	failed         []ImportSkippedEntry
	// favoriteItemIDs are saved items the other password manager starred; they become favorites when the import ends.
	favoriteItemIDs []string
	// content is an encrypted Hisar export waiting for its passphrase; the fields below describe an opened one.
	content      []byte
	notice       string
	isSourceKept bool
	exportedFrom string
	exportedAt   string
}

type ImportSourceGroup struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	IsPrivate bool          `json:"isPrivate"`
	ItemCount int           `json:"itemCount"`
	VaultID   string        `json:"vaultId"`
	VaultName string        `json:"vaultName"`
	Members   []VaultMember `json:"members"`
}

type importDestination struct {
	source  importer.SourceGroup
	vaultID string
	scope   string
}

func (pending *pendingImport) countLeftBehind() int {
	return importer.CountLeftBehind(len(pending.failed), pending.skipped)
}

func (pending *pendingImport) getName() string {
	if pending.filePath == "" {
		return pending.sourceName
	}
	return filepath.Base(pending.filePath)
}

// ListImportFormats lists Hisar's own export first, then the other password managers' formats.
func (s *Service) ListImportFormats() []ImportFormat {
	formats := make([]ImportFormat, 0, len(importer.Formats())+1)
	formats = append(formats, ImportFormat{ID: FormatHisarExport, Name: hisarExportFormatName})
	for _, format := range importer.Formats() {
		formats = append(formats, ImportFormat{ID: format.ID, Name: format.Name})
	}
	return formats
}

// ChooseImportFile asks for an export file and reads it; format "" detects it from the file, Hisar's own export
// included.
func (s *Service) ChooseImportFile(vaultID, format string) (ImportPreview, error) {
	if format != FormatHisarExport && !importer.IsKnownFormat(format) {
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
	defer clear(content)
	if format == FormatHisarExport || format == importer.FormatAutomatic {
		hisarFormat, err := export.DetectFormat(content)
		if err == nil {
			return s.stageHisarExport(&pendingImport{filePath: path, format: hisarFormat}, content)
		}
		if format == FormatHisarExport || !errors.Is(err, export.ErrNotExport) {
			return ImportPreview{}, err
		}
	}
	result, err := importer.Parse(filepath.Base(path), content, format)
	if err != nil {
		return ImportPreview{}, err
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ImportPreview{}, err
	}
	s.dropPendingImport()
	s.pendingImport = &pendingImport{filePath: path, format: result.Format, items: result.Items, skipped: result.Skipped}
	return s.prepareImportGroups(result.SourceGroups(), nil)
}

// PrepareCredentialExchangeImport reads the Credential Exchange (CXF) JSON another password manager handed over, such
// as ASExportedCredentialData from ASCredentialImportManager on iOS or the responseJson of Android's credential transfer,
// and previews it for vaultID like a chosen file; StartImport and ContinueImport then save it. Not a method, so the
// desktop UI cannot call it.
func PrepareCredentialExchangeImport(s *Service, vaultID string, content []byte) (ImportPreview, error) {
	if len(content) > maxImportFileSize {
		return ImportPreview{}, errors.New("there are more than 100 MB of credentials; export fewer at a time")
	}
	if err := s.checkEditableVault(vaultID); err != nil {
		return ImportPreview{}, err
	}
	result, err := importer.ParseCredentialExchange(content)
	if err != nil {
		return ImportPreview{}, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ImportPreview{}, err
	}
	sourceName := result.Source
	if sourceName == "" {
		sourceName = importer.GetFormatName(result.Format)
	}
	s.dropPendingImport()
	s.pendingImport = &pendingImport{sourceName: sourceName, format: result.Format, items: result.Items, skipped: result.Skipped}
	return s.prepareImportGroups(result.SourceGroups(), nil)
}

// readImportFile reads at most maxImportFileSize bytes: a pipe or a file that grows says nothing true about its size
// beforehand.
func readImportFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open %s", filepath.Base(path))
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxImportFileSize+1))
	if err != nil {
		clear(content)
		return nil, fmt.Errorf("could not read %s", filepath.Base(path))
	}
	if len(content) > maxImportFileSize {
		clear(content)
		return nil, errors.New("the file is larger than 100 MB; export without attachments and try again")
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
	if err := s.requireReviewableImport(); err != nil {
		return ImportPreview{}, err
	}
	if len(s.pendingImport.groups) > 1 {
		return ImportPreview{}, errors.New("choose a destination for each source group")
	}
	previous := ""
	if len(s.pendingImport.groups) == 1 {
		previous = s.pendingImport.groups[0].vaultID
		s.pendingImport.groups[0].vaultID = vaultID
	}
	preview, err := s.buildImportPreview(vaultID)
	if err != nil && len(s.pendingImport.groups) == 1 {
		s.pendingImport.groups[0].vaultID = previous
	}
	return preview, err
}

func (s *Service) loadEditableVaultItem(ctx context.Context, vaultID string) (*session.Vault, []item.Item, error) {
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return nil, nil, err
	}
	if vault.Role == apitypes.VaultReader {
		return nil, nil, errReadOnlyVault
	}
	read, err := s.readVaultItems(ctx, vault)
	if err != nil {
		return nil, nil, err
	}
	if !s.isItemListComplete(vault.ID) {
		return nil, nil, session.ErrIncompleteRead
	}
	return vault, read.Items, nil
}

func (s *Service) buildSingleImportPreview(vaultID string) (ImportPreview, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, existingItems, err := s.loadEditableVaultItem(ctx, vaultID)
	if err != nil {
		return ImportPreview{}, err
	}
	pending := s.pendingImport
	summary := importer.Summarize(pending.items)
	preview := ImportPreview{
		FileName:             pending.getName(),
		FilePath:             pending.filePath,
		Format:               pending.format,
		FormatName:           getImportFormatName(pending.format),
		VaultID:              vault.ID,
		VaultName:            vault.Name,
		ItemCount:            len(pending.items),
		LoginCount:           summary.LoginCount,
		SecureNoteCount:      summary.SecureNoteCount,
		SSHKeyCount:          summary.SSHKeyCount,
		CreditCardCount:      summary.CreditCardCount,
		IdentityCount:        summary.IdentityCount,
		APICredentialCount:   summary.APICredentialCount,
		AddressCount:         summary.AddressCount,
		SoftwareLicenseCount: summary.SoftwareLicenseCount,
		ServerCount:          summary.ServerCount,
		DatabaseCount:        summary.DatabaseCount,
		Skipped:              []ImportSkippedEntry{},
		Duplicates:           []ImportDuplicate{},
		Notice:               pending.notice,
		IsSourceKept:         pending.isSourceKept,
		ExportedFrom:         pending.exportedFrom,
		ExportedAt:           pending.exportedAt,
	}
	for _, vaultItem := range pending.items {
		preview.PasskeyCount += len(vaultItem.Details.Passkeys)
	}
	for _, skipped := range pending.skipped {
		preview.Skipped = append(preview.Skipped, ImportSkippedEntry{Title: skipped.Title, Reason: skipped.Reason})
	}
	existingPasskeyIDs := collectPasskeyCredentialIDs(existingItems)
	duplicateMatches := importer.FindDuplicateMatches(pending.items, existingItems)
	for _, index := range importer.FindDuplicateIndexes(pending.items, existingItems) {
		duplicate := pending.items[index]
		firstURL := ""
		if len(duplicate.Overview.URLs) > 0 {
			firstURL = duplicate.Overview.URLs[0]
		}
		passkeyCount := 0
		existing := existingItems[duplicateMatches[index]]
		if existing.IsCategory(item.CategoryLogin) {
			passkeyCount = len(filterNewPasskeys(duplicate.Details.Passkeys, existingPasskeyIDs))
		}
		preview.Duplicates = append(preview.Duplicates, ImportDuplicate{
			Title: duplicate.Overview.Title, Username: duplicate.Overview.Username, URL: firstURL, Detail: importer.DescribeDuplicateOf(duplicate, existing),
			PasskeyCount: passkeyCount,
		})
	}
	return preview, nil
}

// collectPasskeyCredentialIDs returns the credential IDs of the passkeys items hold.
func collectPasskeyCredentialIDs(items []item.Item) map[string]bool {
	credentialIDs := map[string]bool{}
	for _, vaultItem := range items {
		for _, storedPasskey := range vaultItem.Details.Passkeys {
			credentialIDs[string(storedPasskey.CredentialID)] = true
		}
	}
	return credentialIDs
}

// filterNewPasskeys returns the passkeys whose credential ID is not in credentialIDs.
func filterNewPasskeys(passkeys []item.Passkey, credentialIDs map[string]bool) []item.Passkey {
	var newPasskeys []item.Passkey
	for _, importedPasskey := range passkeys {
		if !credentialIDs[string(importedPasskey.CredentialID)] {
			newPasskeys = append(newPasskeys, importedPasskey)
		}
	}
	return newPasskeys
}

// StartImport queues the previewed items for vaultID; ContinueImport then saves them in batches. A passkey the vault
// already has is left out, so importing again adds none twice. Skipping duplicates still adds a duplicate login's new
// passkeys to the login the vault has, so passkeys exported after their logins (Credential Exchange after a .1pux) arrive.
func (s *Service) StartImport(vaultID string, isSkippingDuplicate bool) (ImportProgress, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ImportProgress{}, err
	}
	if err := s.requireReviewableImport(); err != nil {
		return ImportProgress{}, err
	}
	return s.startImportGroups(vaultID, isSkippingDuplicate)
}

func (s *Service) startSingleImport(pending *pendingImport, vaultID string, isSkippingDuplicate bool) (ImportProgress, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, existingItems, err := s.loadEditableVaultItem(ctx, vaultID)
	if err != nil {
		return ImportProgress{}, err
	}
	duplicateMatches := map[int]int{}
	if isSkippingDuplicate {
		duplicateMatches = importer.FindDuplicateMatches(pending.items, existingItems)
	}
	existingPasskeyIDs := collectPasskeyCredentialIDs(existingItems)
	queuedItems := make([]item.Item, 0, len(pending.items))
	// mergedPositions maps an existing login that receives passkeys to its place in queuedItems, so two imported items
	// that match it make one update.
	mergedPositions := map[int]int{}
	for index, vaultItem := range pending.items {
		newPasskeys := filterNewPasskeys(vaultItem.Details.Passkeys, existingPasskeyIDs)
		for _, newPasskey := range newPasskeys {
			existingPasskeyIDs[string(newPasskey.CredentialID)] = true
		}
		existingIndex, isDuplicate := duplicateMatches[index]
		switch {
		case !isDuplicate:
			vaultItem.Details.Passkeys = newPasskeys
			item.UpdatePasskeyOverview(&vaultItem)
			queuedItems = append(queuedItems, vaultItem)
		case len(newPasskeys) == 0 || !existingItems[existingIndex].IsCategory(item.CategoryLogin):
			// A plain duplicate is skipped.
		default:
			position, isMerged := mergedPositions[existingIndex]
			if !isMerged {
				merged := existingItems[existingIndex]
				merged.Details.Passkeys, merged.IsFavorite = slices.Clone(merged.Details.Passkeys), false
				position = len(queuedItems)
				mergedPositions[existingIndex] = position
				queuedItems = append(queuedItems, merged)
			}
			merged := &queuedItems[position]
			merged.Details.Passkeys = append(merged.Details.Passkeys, newPasskeys...)
			item.UpdatePasskeyOverview(merged)
		}
	}
	if len(mergedPositions) > 0 {
		existingItems = slices.Clone(existingItems)
		for existingIndex, position := range mergedPositions {
			existingItems[existingIndex] = queuedItems[position]
		}
	}
	pending.skipped = append(pending.skipped, importer.FindUnsavedDuplicates(pending.items, existingItems, duplicateMatches)...)
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
	if len(pending.queuedVaultIDs) == 0 && len(pending.queuedItems) > 0 {
		return ImportProgress{}, errors.New("review the import destinations again")
	}
	for batchCount := 0; batchCount < importBatchSize && len(pending.queuedItems) > 0; batchCount++ {
		vaultID := pending.queuedVaultIDs[0]
		vault, err := s.findVault(ctx, vaultID)
		if err != nil {
			return ImportProgress{}, err
		}
		vaultItem := pending.queuedItems[0]
		err = s.saveSessionItem(ctx, vault, &vaultItem, pending.scopes[vaultID])
		if isImportStoppingError(err) {
			return ImportProgress{}, convertError(err)
		}
		pending.queuedItems = pending.queuedItems[1:]
		pending.queuedVaultIDs = pending.queuedVaultIDs[1:]
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
		errors.Is(err, session.ErrConflict) || errors.Is(err, session.ErrImportScopeChanged) || errors.Is(err, apiclient.ErrAccountSuspended) || apiclient.IsStatus(err, http.StatusForbidden) || apiclient.IsStatus(err, http.StatusTooManyRequests) || errors.Is(err, context.DeadlineExceeded))
}

func (s *Service) buildImportProgress() ImportProgress {
	pending := s.pendingImport
	progress := ImportProgress{
		FileName:        pending.getName(),
		FilePath:        pending.filePath,
		VaultID:         pending.vaultID,
		SavedCount:      pending.savedCount,
		TotalCount:      pending.savedCount + len(pending.failed) + len(pending.queuedItems),
		LeftBehindCount: pending.countLeftBehind(),
		IsDone:          len(pending.queuedItems) == 0,
		Failed:          slices.Clone(pending.failed),
		IsSourceKept:    pending.isSourceKept,
	}
	if vault, found := s.vaultsByID[pending.vaultID]; found {
		progress.VaultName = vault.Name
	} else {
		progress.VaultName = "the mapped vaults"
	}
	return progress
}

func (s *Service) CancelImport() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.dropPendingImport()
}

// dropPendingImport forgets the import and wipes an encrypted export still waiting for its passphrase. It needs the
// mutex.
func (s *Service) dropPendingImport() {
	if s.pendingImport != nil {
		clear(s.pendingImport.content)
	}
	s.pendingImport = nil
}

// DeleteImportFile removes the export file after a finished import, since it holds every password in plain text. It
// refuses while the file holds something Hisar did not save, an item that failed or an entry it skipped.
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
	if leftCount := pending.countLeftBehind(); leftCount > 0 {
		return fmt.Errorf("keep %s: %d of its entries are not in Hisar", pending.getName(), leftCount)
	}
	if pending.isSourceKept {
		return fmt.Errorf("keep %s: Hisar never deletes an encrypted export or a copy that may not hold everything", pending.getName())
	}
	if pending.filePath != "" {
		if err := os.Remove(pending.filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("could not delete %s; delete it in Finder", filepath.Base(pending.filePath))
		}
	}
	s.pendingImport = nil
	return nil
}
