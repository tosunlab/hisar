package signin

import (
	"slices"
	"strings"
	"unicode"
)

type form struct {
	frame    int
	username *Field
	password *Field
	code     []Field
	// submit is nil when the frame has no sign-in button; Return in the last field submits then.
	submit *Button
}

func (step form) fieldsOf(part Part) []ElementID {
	switch {
	case part == PartUsername && step.username != nil:
		return []ElementID{step.username.ID}
	case part == PartPassword && step.password != nil:
		return []ElementID{step.password.ID}
	case part == PartCode:
		ids := []ElementID{}
		for _, field := range step.code {
			ids = append(ids, field.ID)
		}
		return ids
	}
	return nil
}

func (step form) asks(part Part) bool {
	return len(step.fieldsOf(part)) > 0
}

func (step form) fieldIDs() []ElementID {
	return slices.Concat(step.fieldsOf(PartUsername), step.fieldsOf(PartPassword), step.fieldsOf(PartCode))
}

var (
	passwordWords     = []string{"password", "passwort", "contraseña", "mot de passe", "wachtwoord", "senha"}
	codeWords         = []string{"code", "otp", "one-time", "2fa", "mfa", "totp", "verification", "authenticator", "digit", "token"}
	notCodeWords      = []string{"postal", "zip", "post code", "promo", "coupon", "discount", "country", "area", "invite", "referral", "gift"}
	usernameWords     = []string{"user", "email", "e-mail", "login", "account", "identifier", "phone", "mobile"}
	signInWords       = []string{"sign in", "signin", "log in", "login", "log on", "sign on", "next", "continue", "submit", "verify", "confirm", "authenticate"}
	notSignInWords    = []string{" with ", "passkey", "sso", "sign up", "register", "create", "forgot", "magic", "send", "subscribe"}
	minimumCodeDigits = 4
	maximumCodeDigits = 8
)

func containsAny(text string, words []string) bool {
	return slices.ContainsFunc(words, func(word string) bool { return strings.Contains(text, word) })
}

func describe(field Field) string {
	return strings.ToLower(field.Label + " " + field.Identifier + " " + field.Placeholder)
}

func isCodeField(field Field) bool {
	words := describe(field)
	return !field.IsSecure && containsAny(words, codeWords) && !containsAny(words, notCodeWords)
}

func isUsernameField(field Field) bool {
	return strings.Contains(strings.ToLower(field.InputType), "email") || containsAny(describe(field), usernameWords)
}

func normalizeTitle(title string) string {
	trimmed := strings.TrimFunc(strings.ToLower(title), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	return strings.Join(strings.Fields(trimmed), " ")
}

func isSignInButton(button Button) bool {
	title := normalizeTitle(button.Title)
	if title == "" || containsAny(" "+title+" ", notSignInWords) {
		return false
	}
	return slices.ContainsFunc(signInWords, func(word string) bool { return title == word || strings.HasPrefix(title, word+" ") })
}

// findForm finds the sign-in step the page shows: in the first frame, in document order, that holds a password field, a
// one-time code field or a username field with a sign-in button. It refuses a frame with two password fields, as
// sign-up and change-password forms have, and one with a field that calls itself a password but is not a secure text
// field. Only visible, editable fields count.
func findForm(page Page) (form, Reason, bool) {
	for frame := range page.Frames {
		fields := []Field{}
		for _, field := range page.Fields {
			if field.Frame == frame && field.IsVisible && field.IsEditable {
				fields = append(fields, field)
			}
		}
		step, reason, isFound := classifyFrame(frame, fields, page.Buttons)
		if isFound || reason != "" {
			return step, reason, isFound
		}
	}
	return form{}, ReasonNoForm, false
}

func classifyFrame(frame int, fields []Field, buttons []Button) (form, Reason, bool) {
	step := form{frame: frame}
	for index, button := range buttons {
		if button.Frame == frame && button.IsVisible && isSignInButton(button) {
			step.submit = &buttons[index]
			break
		}
	}
	secureCount := 0
	for index, field := range fields {
		switch {
		case field.IsSecure:
			secureCount++
			if step.password == nil {
				step.password = &fields[index]
				step.username = findUsernameBefore(fields[:index])
			}
		case containsAny(describe(field), passwordWords):
			return form{}, ReasonNotSecureField, false
		}
	}
	if secureCount > 1 {
		return form{}, ReasonSignUpForm, false
	}
	step.code = findCode(fields)
	if step.password != nil || len(step.code) > 0 {
		return step, "", true
	}
	if step.submit == nil {
		return form{}, "", false
	}
	if username := findLoneUsername(fields); username != nil {
		step.username = username
		return step, "", true
	}
	return form{}, "", false
}

func findUsernameBefore(fields []Field) *Field {
	for index := len(fields) - 1; index >= 0; index-- {
		if !fields[index].IsSecure && !isCodeField(fields[index]) {
			return &fields[index]
		}
	}
	return nil
}

func findLoneUsername(fields []Field) *Field {
	candidates := []Field{}
	for _, field := range fields {
		if !field.IsSecure && !isCodeField(field) {
			candidates = append(candidates, field)
		}
	}
	for index, field := range candidates {
		if isUsernameField(field) {
			return &candidates[index]
		}
	}
	if len(candidates) == 1 {
		return &candidates[0]
	}
	return nil
}

func findCode(fields []Field) []Field {
	var single []Field
	for start := 0; start < len(fields); start++ {
		end := start
		for end < len(fields) && isCodeField(fields[end]) {
			end++
		}
		if count := end - start; count >= minimumCodeDigits && count <= maximumCodeDigits {
			return fields[start:end]
		}
		if end > start && single == nil {
			single = fields[start : start+1]
		}
		start = max(start, end)
	}
	return single
}

// checkFrames reads the origin of the form's frame and requires a website on a secure origin whose every ancestor frame
// has the same origin, as a sign-in form framed by another site does not.
func checkFrames(page Page, frame int) (Origin, *Refusal) {
	origin, err := ParseOrigin(page.Frames[frame].URL)
	if err != nil {
		return Origin{}, refuseNotWebPage(page.Frames[frame].URL)
	}
	if !origin.isSecure() {
		return Origin{}, &Refusal{Reason: ReasonInsecure, Message: "The page is " + origin.String() + ". Hisar signs in only on https pages."}
	}
	for parent := page.Frames[frame].Parent; parent >= 0; parent = page.Frames[parent].Parent {
		if ancestor, err := ParseOrigin(page.Frames[parent].URL); err != nil || ancestor != origin {
			return Origin{}, &Refusal{Reason: ReasonCrossOriginFrame, Message: "The sign-in form comes from " + origin.String() +
				" inside a page of " + describeURL(page.Frames[0].URL) + ". Open " + origin.String() + " itself and call sign_in again."}
		}
	}
	return origin, nil
}
