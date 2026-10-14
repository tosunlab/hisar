package app

import (
	"errors"
	"slices"
)

// Theme is this device's look: Appearance follows the system or picks light or dark, Color tints the chrome and accent.
type Theme struct {
	Appearance string `json:"appearance"`
	Color      string `json:"color"`
}

const (
	defaultAppearance = "system"
	defaultThemeColor = "neutral"
)

var (
	appearances = []string{"system", "light", "dark"}
	themeColors = []string{"neutral", "blue", "purple", "green"}
)

func (s *Service) GetTheme() Theme {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.buildTheme()
}

func (s *Service) buildTheme() Theme {
	theme := Theme{Appearance: defaultAppearance, Color: defaultThemeColor}
	if slices.Contains(appearances, s.state.Appearance) {
		theme.Appearance = s.state.Appearance
	}
	if slices.Contains(themeColors, s.state.Color) {
		theme.Color = s.state.Color
	}
	return theme
}

// SetTheme saves the theme for this device; it is kept when signing out.
func (s *Service) SetTheme(appearance, color string) (Theme, error) {
	if !slices.Contains(appearances, appearance) {
		return Theme{}, errors.New("choose System, Light or Dark")
	}
	if !slices.Contains(themeColors, color) {
		return Theme{}, errors.New("choose Neutral, Blue, Purple or Green")
	}
	s.mutex.Lock()
	s.state.Appearance, s.state.Color = appearance, color
	err := s.state.Save()
	theme := s.buildTheme()
	s.mutex.Unlock()
	if err != nil {
		return theme, err
	}
	if s.config.OnThemeChange != nil {
		s.config.OnThemeChange(theme)
	}
	return theme, nil
}
