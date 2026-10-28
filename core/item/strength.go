package item

import (
	"math"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	// minimumMasterPasswordLength counts characters, as session.ChangeMasterPassword does.
	minimumMasterPasswordLength = 10
	// Below this the apps refuse a master password; the meter still says Weak up to 40 bits.
	minimumMasterPasswordBits = 28
	bitsPerDictionaryWord     = 11
	commonPatternPenalty      = 20
	masterPasswordAdvice      = "A few random words you can remember make a strong master password."
)

// PasswordStrength is a password's estimated entropy and the four-segment meter's rating of it. Level is "" for an
// empty password, else "weak", "fair", "good" or "strong"; Label is the word the meter shows.
type PasswordStrength struct {
	Level string `json:"level"`
	Label string `json:"label"`
	Bits  int    `json:"bits"`
}

// MasterPasswordFeedback is the rating of a new master password and the one sentence under the meter. A blocking
// message refuses the password: fewer than 10 characters, or an estimate below 28 bits.
type MasterPasswordFeedback struct {
	Level      string `json:"level"`
	Label      string `json:"label"`
	Bits       int    `json:"bits"`
	Message    string `json:"message"`
	IsBlocking bool   `json:"isBlocking"`
}

// CheckMasterPassword is the apps' one rule for a new master password, on Create account and Change master password.
func CheckMasterPassword(password string) MasterPasswordFeedback {
	strength := RatePassword(password)
	feedback := MasterPasswordFeedback{Level: strength.Level, Label: strength.Label, Bits: strength.Bits, Message: masterPasswordAdvice}
	switch {
	case password == "":
	case utf8.RuneCountInString(password) < minimumMasterPasswordLength:
		feedback.Message, feedback.IsBlocking = "Use at least 10 characters.", true
	case strength.Bits < minimumMasterPasswordBits:
		feedback.Message, feedback.IsBlocking = "This password is too easy to guess. Add a few more words or characters.", true
	case strength.Level == "weak":
		feedback.Message = "You can use it, but another word or a few more characters make it much harder to guess."
	}
	return feedback
}

// RatePassword estimates a password's entropy: a phrase of letter words counts per word, common words and patterns
// cost 20 bits each, and otherwise the character classes in use count per character, repeats and sequences less.
// It works on UTF-16 code units, as the Mac's first version in JavaScript did.
func RatePassword(password string) PasswordStrength {
	if password == "" {
		return PasswordStrength{}
	}
	penalty := countCommonPatterns(password) * commonPatternPenalty
	if wordCount := countPhraseWords(password); wordCount > 1 {
		return classifyBits(wordCount*bitsPerDictionaryWord - penalty)
	}
	var lowerSize, upperSize, digitSize, otherSize int
	units := utf16.Encode([]rune(password))
	effectiveLength := 0.0
	for index, unit := range units {
		switch {
		case unit >= 'a' && unit <= 'z':
			lowerSize = 26
		case unit >= 'A' && unit <= 'Z':
			upperSize = 26
		case unit >= '0' && unit <= '9':
			digitSize = 10
		default:
			otherSize = 33
		}
		if index > 0 && max(unit, units[index-1])-min(unit, units[index-1]) <= 1 {
			effectiveLength += 0.3
		} else {
			effectiveLength++
		}
	}
	poolSize := max(lowerSize+upperSize+digitSize+otherSize, 2)
	return classifyBits(int(math.Round(effectiveLength*math.Log2(float64(poolSize)))) - penalty)
}

func countCommonPatterns(password string) int {
	lower := toLowerASCII(password)
	count := 0
	for _, word := range []string{"pass", "qwert", "admin", "welcome", "letmein", "iloveyou"} {
		if strings.Contains(lower, word) {
			count++
		}
	}
	for _, season := range []string{"spring", "summer", "autumn", "fall", "winter"} {
		if strings.Contains(lower, season) {
			count++
			break
		}
	}
	if containsYear(password) {
		count++
	}
	for _, sequence := range []string{"1234", "abc"} {
		if strings.Contains(lower, sequence) {
			count++
		}
	}
	return count
}

// countPhraseWords is the number of words in letters-only words joined by - _ space or dot, or 0 for anything else.
func countPhraseWords(password string) int {
	wordCount, wordLength := 1, 0
	for index := range len(password) {
		switch character := password[index]; {
		case character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z':
			wordLength++
		case strings.IndexByte("-_ .", character) >= 0 && wordLength > 0:
			wordCount, wordLength = wordCount+1, 0
		default:
			return 0
		}
	}
	if wordLength == 0 {
		return 0
	}
	return wordCount
}

// containsYear finds 19 or 20 followed by two more digits, as a year from 1900 to 2099.
func containsYear(password string) bool {
	for index := range len(password) - 3 {
		century := password[index : index+2]
		if (century == "19" || century == "20") && isDigit(password[index+2]) && isDigit(password[index+3]) {
			return true
		}
	}
	return false
}

func isDigit(character byte) bool {
	return character >= '0' && character <= '9'
}

// toLowerASCII lowers only A to Z, as JavaScript's case-insensitive match of these ASCII patterns does.
func toLowerASCII(text string) string {
	return strings.Map(func(character rune) rune {
		if character >= 'A' && character <= 'Z' {
			return character + 'a' - 'A'
		}
		return character
	}, text)
}

func classifyBits(estimatedBits int) PasswordStrength {
	bits := max(estimatedBits, 0)
	switch {
	case bits < 40:
		return PasswordStrength{Level: "weak", Label: "Weak", Bits: bits}
	case bits < 60:
		return PasswordStrength{Level: "fair", Label: "Fair", Bits: bits}
	case bits < 90:
		return PasswordStrength{Level: "good", Label: "Good", Bits: bits}
	default:
		return PasswordStrength{Level: "strong", Label: "Strong", Bits: bits}
	}
}
