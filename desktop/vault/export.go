package vault

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/export"
)

const (
	ExportFormatEncrypted = cliprotocol.ExportFormatEncrypted
	ExportFormatPlain     = cliprotocol.ExportFormatPlain
)

var (
	errUnknownExportFormat = errors.New("choose an encrypted export or plain JSON")
	errExportNeedsPassword = errors.New("enter your master password to export")
	errExportFileExists    = errors.New("a file with that name already exists; choose another name")
)

// ExportOptions tells the Settings screen whether Touch ID can approve an export or the master password is needed.
type ExportOptions struct {
	IsBiometricAvailable bool `json:"isBiometricAvailable"`
	MinPassphraseLength  int  `json:"minPassphraseLength"`
}

// ExportRequest: Passphrase protects an encrypted export; Password is the master password when Touch ID is unavailable.
type ExportRequest struct {
	Format     string `json:"format"`
	Passphrase string `json:"passphrase"`
	Password   string `json:"password"`
}

type ExportResult struct {
	Path       string `json:"path"`
	VaultCount int    `json:"vaultCount"`
	ItemCount  int    `json:"itemCount"`
}

func (s *Service) GetExportOptions() ExportOptions {
	return ExportOptions{IsBiometricAvailable: s.isTouchIDAvailable(), MinPassphraseLength: export.MinPassphraseLength}
}

func (s *Service) isTouchIDAvailable() bool {
	return s.config.Biometric != nil && s.config.Biometric.IsAvailable()
}

func checkExportRequest(format, passphrase string) error {
	switch format {
	case ExportFormatEncrypted:
		return export.CheckPassphrase([]byte(passphrase))
	case ExportFormatPlain:
		return nil
	}
	return errUnknownExportFormat
}

// ExportAllData asks for Touch ID or the master password, then saves every vault the user can read where they choose.
// Nothing about plans or the server's answer limits it: offline it exports this device's encrypted cache.
func (s *Service) ExportAllData(request ExportRequest) (ExportResult, error) {
	if err := checkExportRequest(request.Format, request.Passphrase); err != nil {
		return ExportResult{}, err
	}
	if err := s.authenticateExport(request.Password); err != nil {
		return ExportResult{}, err
	}
	content, result, err := s.buildExport(request.Format, request.Passphrase)
	if err != nil {
		return ExportResult{}, err
	}
	defer clear(content)
	if s.config.FileSaver == nil {
		return ExportResult{}, errors.New("saving files is not available")
	}
	path, err := s.config.FileSaver.ChooseSavePath(buildExportFileName(request.Format, s.config.Now()))
	if err != nil || path == "" {
		return ExportResult{}, err
	}
	if err := writeFileAtomically(path, content); err != nil {
		return ExportResult{}, err
	}
	result.Path = path
	return result, nil
}

func (s *Service) authenticateExport(password string) error {
	s.mutex.Lock()
	err := s.requireUnlocked()
	s.mutex.Unlock()
	if err != nil {
		return err
	}
	switch {
	case password != "":
		_, err := s.loadVerifiedSecretKey(password)
		return err
	case s.isTouchIDAvailable():
		return s.config.Biometric.Authenticate("export your Hisar data")
	}
	return errExportNeedsPassword
}

// ExportAllDataToDirectory is for platforms that share the file themselves, such as the iPhone; a function rather than
// a method so the desktop UI, which saves through ExportAllData, can't call it.
func ExportAllDataToDirectory(s *Service, request ExportRequest, directory string) (ExportResult, error) {
	if err := checkExportRequest(request.Format, request.Passphrase); err != nil {
		return ExportResult{}, err
	}
	if err := s.authenticateExport(request.Password); err != nil {
		return ExportResult{}, err
	}
	content, result, err := s.buildExport(request.Format, request.Passphrase)
	if err != nil {
		return ExportResult{}, err
	}
	defer clear(content)
	path := filepath.Join(directory, buildExportFileName(request.Format, s.config.Now()))
	if err := writeFileAtomically(path, content); err != nil {
		return ExportResult{}, err
	}
	result.Path = path
	return result, nil
}

// writeExportFile is for the command-line tool, whose approval dialog already asked: it never replaces a file.
func (s *Service) writeExportFile(format, passphrase, path string) (ExportResult, error) {
	if err := checkExportRequest(format, passphrase); err != nil {
		return ExportResult{}, err
	}
	if !filepath.IsAbs(path) {
		return ExportResult{}, errors.New("the export path must be absolute")
	}
	content, result, err := s.buildExport(format, passphrase)
	if err != nil {
		return ExportResult{}, err
	}
	defer clear(content)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return ExportResult{}, errExportFileExists
	}
	if err != nil {
		return ExportResult{}, fmt.Errorf("could not save the export: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		os.Remove(path)
		return ExportResult{}, fmt.Errorf("could not save the export: %w", err)
	}
	if err := file.Close(); err != nil {
		return ExportResult{}, fmt.Errorf("could not save the export: %w", err)
	}
	result.Path = path
	return result, nil
}

func (s *Service) buildExport(format, passphrase string) ([]byte, ExportResult, error) {
	s.mutex.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mutex.Unlock()
		return nil, ExportResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	allVault, err := export.Collect(ctx, s.unlockedSession)
	account := export.Account{Email: s.state.Email, Server: s.state.Server}
	s.mutex.Unlock()
	if err != nil {
		return nil, ExportResult{}, convertError(err)
	}
	document := export.Build(account, allVault, s.config.Now())
	result := ExportResult{VaultCount: len(document.Vaults), ItemCount: document.CountItem()}
	if format == ExportFormatPlain {
		content, err := export.EncodePlain(document)
		return content, result, err
	}
	passphraseBytes := []byte(passphrase)
	defer clear(passphraseBytes)
	content, err := export.EncodeEncrypted(document, passphraseBytes)
	return content, result, err
}

func buildExportFileName(format string, now time.Time) string {
	if format == ExportFormatPlain {
		return "Hisar export " + now.Format("2006-01-02") + " (not encrypted).json"
	}
	return "Hisar export " + now.Format("2006-01-02") + ".hisar"
}

func writeFileAtomically(path string, content []byte) error {
	temporaryFile, err := os.CreateTemp(filepath.Dir(path), ".hisar-export-*")
	if err != nil {
		return fmt.Errorf("could not save the export: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporaryFile.Write(content); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("could not save the export: %w", err)
	}
	if err := temporaryFile.Sync(); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("could not save the export: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("could not save the export: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("could not save the export: %w", err)
	}
	return nil
}
