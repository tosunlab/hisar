package app

import (
	"errors"
	"fmt"
	"time"
)

// CopyField copies an item's value and clears the clipboard later if it still holds that value.
func (s *Service) CopyField(vaultID, itemID, fieldName string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	vaultItem, err := s.findItem(vaultID, itemID)
	if err != nil {
		return err
	}
	value, found := getFieldValue(vaultItem, fieldName)
	if !found || value == "" {
		return fmt.Errorf("field %q %w", fieldName, ErrNotFound)
	}
	return s.copyText(value)
}

func (s *Service) CopyText(text string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.copyText(text)
}

func (s *Service) copyText(text string) error {
	if s.config.Clipboard == nil || !s.config.Clipboard.SetText(text) {
		return errors.New("could not copy to the clipboard")
	}
	if s.clipboardClearTimer != nil {
		s.clipboardClearTimer.Stop()
	}
	clipboard := s.config.Clipboard
	s.clipboardClearTimer = time.AfterFunc(s.config.ClipboardClearAfter, func() {
		if current, ok := clipboard.GetText(); ok && current == text {
			clipboard.SetText("")
		}
	})
	return nil
}
