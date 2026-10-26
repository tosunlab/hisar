package app

import (
	"bytes"
	"errors"
	"time"

	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/importer"
	"github.com/tryhisar/hisar/core/item"
)

// FormatHisarExport is the import format for Hisar's own export (§9.11), plain or encrypted; the file says which.
const FormatHisarExport = "hisar"

const hisarExportFormatName = "Hisar export (.hisar or .json)"

func getImportFormatName(format string) string {
	switch format {
	case export.FormatEncrypted:
		return "Hisar encrypted export"
	case export.FormatPlain:
		return "Hisar export (plain JSON)"
	}
	return importer.GetFormatName(format)
}

// requireReviewableImport: an import chosen and not started, and not an encrypted export still waiting for its
// passphrase. It needs the mutex.
func (s *Service) requireReviewableImport() error {
	if s.pendingImport == nil || s.pendingImport.isStarted {
		return ErrNoPendingImport
	}
	if s.pendingImport.content != nil {
		return export.ErrPassphraseRequired
	}
	return nil
}

// PrepareHisarExportImport reads a Hisar export the phone's file picker handed over and previews it like a chosen file.
// An encrypted export comes back with IsPassphraseRequired, and OpenImportFile opens it; StartImport and ContinueImport
// then save it. Not a method, so the desktop UI cannot call it.
func PrepareHisarExportImport(s *Service, vaultID, fileName string, content []byte) (ImportPreview, error) {
	if len(content) > maxImportFileSize {
		return ImportPreview{}, errors.New("the file is larger than 100 MB")
	}
	if err := s.checkEditableVault(vaultID); err != nil {
		return ImportPreview{}, err
	}
	format, err := export.DetectFormat(content)
	if err != nil {
		return ImportPreview{}, err
	}
	return s.stageHisarExport(&pendingImport{sourceName: fileName, format: format}, content)
}

// stageHisarExport makes pending the import. A plain export is opened at once; an encrypted one keeps a copy of content
// until OpenImportFile gets its passphrase. The caller keeps and clears content.
func (s *Service) stageHisarExport(pending *pendingImport, content []byte) (ImportPreview, error) {
	if pending.format == export.FormatPlain {
		document, err := export.Decode(content, nil)
		if err != nil {
			return ImportPreview{}, err
		}
		s.mutex.Lock()
		defer s.mutex.Unlock()
		if err := s.requireUnlocked(); err != nil {
			return ImportPreview{}, err
		}
		s.dropPendingImport()
		s.pendingImport = pending
		return s.openHisarExport(document)
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ImportPreview{}, err
	}
	s.dropPendingImport()
	pending.content = bytes.Clone(content)
	s.pendingImport = pending
	return ImportPreview{FileName: pending.getName(), FilePath: pending.filePath, Format: pending.format, FormatName: getImportFormatName(pending.format), IsPassphraseRequired: true}, nil
}

// OpenImportFile opens the encrypted Hisar export ChooseImportFile or PrepareHisarExportImport read, and previews it.
// A wrong passphrase keeps the file waiting, so the user can try again.
func (s *Service) OpenImportFile(passphrase string) (ImportPreview, error) {
	s.mutex.Lock()
	pending := s.pendingImport
	if err := s.requireUnlocked(); err != nil {
		s.mutex.Unlock()
		return ImportPreview{}, err
	}
	if pending == nil || pending.content == nil {
		s.mutex.Unlock()
		return ImportPreview{}, ErrNoPendingImport
	}
	content := bytes.Clone(pending.content)
	defer clear(content)
	s.mutex.Unlock()

	// Argon2id takes a moment, so the passphrase is tried without holding the service.
	passphraseBytes := []byte(passphrase)
	defer clear(passphraseBytes)
	document, err := export.Decode(content, passphraseBytes)

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ImportPreview{}, err
	}
	if s.pendingImport != pending {
		return ImportPreview{}, ErrNoPendingImport
	}
	if err != nil {
		return ImportPreview{}, err
	}
	clear(pending.content)
	pending.content = nil
	return s.openHisarExport(document)
}

// openHisarExport previews the opened document for s.pendingImport: one group per exported vault, each restored to the
// vault with its ID when this account can write to it. It needs the mutex.
func (s *Service) openHisarExport(document *export.Document) (ImportPreview, error) {
	pending := s.pendingImport
	groups := document.SourceGroups()
	restoredTypes := map[string]string{}
	for _, exported := range document.Vaults {
		if exported.ID != "" {
			restoredTypes[exported.ID] = exported.Type
		}
	}
	pending.items = []item.Item{}
	for _, group := range groups {
		pending.items = append(pending.items, group.Items...)
	}
	pending.notice = document.Provenance.Message()
	pending.isSourceKept = pending.format == export.FormatEncrypted || document.Provenance.IsCachedCopy() || document.Provenance.IsPartial()
	pending.exportedFrom = document.Account.Email
	if !document.ExportedAt.IsZero() {
		pending.exportedAt = document.ExportedAt.UTC().Format(time.RFC3339)
	}
	return s.prepareImportGroups(groups, restoredTypes)
}
