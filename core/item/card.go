package item

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CategoryCreditCard items keep their values in custom fields with the names below, so an app that does not know cards
// shows them as fields and keeps them when it edits the item.
const CategoryCreditCard = "credit-card"

// The custom field names of a card's values.
const (
	FieldCardholder     = "Cardholder"
	FieldCardType       = "Type"
	FieldCardNumber     = "Card number"
	FieldExpiry         = "Expiry"
	FieldSecurityCode   = "Security code"
	FieldPIN            = "PIN"
	FieldBillingAddress = "Billing address"
)

// Card is what a credit-card item holds. Type is the card's brand, such as Visa; saving fills it in from the number when
// it is empty.
type Card struct {
	Cardholder     string
	Type           string
	Number         string
	Expiry         string
	SecurityCode   string
	PIN            string
	BillingAddress string
}

// CardField is one of a card's values: its field name, whether it is concealed, and where it lives in a Card.
type CardField struct {
	Name      string
	Concealed bool
	value     func(*Card) *string
}

// Value points at the field's value in card.
func (field CardField) Value(card *Card) *string { return field.value(card) }

// ListCardFields returns a card's fields in the order they are stored and asked for.
func ListCardFields() []CardField { return slices.Clone(cardFields) }

// cardFields is the order of a card's fields and which of them are concealed.
var cardFields = []CardField{
	{FieldCardholder, false, func(card *Card) *string { return &card.Cardholder }},
	{FieldCardType, false, func(card *Card) *string { return &card.Type }},
	{FieldCardNumber, true, func(card *Card) *string { return &card.Number }},
	{FieldExpiry, false, func(card *Card) *string { return &card.Expiry }},
	{FieldSecurityCode, true, func(card *Card) *string { return &card.SecurityCode }},
	{FieldPIN, true, func(card *Card) *string { return &card.PIN }},
	{FieldBillingAddress, false, func(card *Card) *string { return &card.BillingAddress }},
}

// ReadCard returns the card's values and the item's other custom fields. Names match ignoring case; when a name repeats,
// the first one is the card's and the rest stay other fields.
func ReadCard(vaultItem *Item) (Card, []Field) {
	var card Card
	isRead := map[string]bool{}
	otherFields := []Field{}
	for _, field := range vaultItem.Details.Fields {
		isCardField := false
		for _, cardField := range cardFields {
			if strings.EqualFold(field.Name, cardField.Name) && !isRead[cardField.Name] {
				*cardField.value(&card) = field.Value
				isRead[cardField.Name], isCardField = true, true
				break
			}
		}
		if !isCardField {
			otherFields = append(otherFields, field)
		}
	}
	return card, otherFields
}

// WriteCard stores card as the item's first custom fields, in a fixed order with the secret ones concealed, followed by
// otherFields; an empty value is left out.
func WriteCard(vaultItem *Item, card Card, otherFields []Field) {
	fields := []Field{}
	for _, cardField := range cardFields {
		if value := *cardField.value(&card); value != "" {
			fields = append(fields, Field{Name: cardField.Name, Value: value, Concealed: cardField.Concealed})
		}
	}
	fields = append(fields, otherFields...)
	vaultItem.Details.Fields = nil
	if len(fields) > 0 {
		vaultItem.Details.Fields = fields
	}
}

// NormalizeCard tidies a credit-card item's number, type and expiry and puts its fields in order; other items are left
// alone. An empty type is filled in from the number.
func NormalizeCard(vaultItem *Item) {
	if !vaultItem.IsCategory(CategoryCreditCard) {
		return
	}
	card, otherFields := ReadCard(vaultItem)
	card.Cardholder = strings.TrimSpace(card.Cardholder)
	card.Number = FormatCardNumber(card.Number)
	if card.Type = NormalizeCardType(card.Type); card.Type == "" {
		card.Type = GetCardBrand(card.Number)
	}
	if expiry, isValid := NormalizeExpiry(card.Expiry); isValid {
		card.Expiry = expiry
	} else {
		card.Expiry = strings.TrimSpace(card.Expiry)
	}
	card.SecurityCode, card.PIN = strings.TrimSpace(card.SecurityCode), strings.TrimSpace(card.PIN)
	card.BillingAddress = strings.TrimSpace(card.BillingAddress)
	WriteCard(vaultItem, card, otherFields)
}

var cardNumberCharacters = regexp.MustCompile(`^[0-9 -]+$`)

// getCardDigits returns the digits of a number written with digits, spaces and dashes, and "" for anything else.
func getCardDigits(number string) string {
	number = strings.TrimSpace(number)
	if !cardNumberCharacters.MatchString(number) {
		return ""
	}
	return strings.NewReplacer(" ", "", "-", "").Replace(number)
}

// FormatCardNumber groups a card number's digits as printed on the card; a number with other characters is kept as typed.
func FormatCardNumber(number string) string {
	digits := getCardDigits(number)
	if digits == "" {
		return strings.TrimSpace(number)
	}
	groupSizes := []int{4, 4, 4, 4, 4}
	switch brand := GetCardBrand(digits); {
	case brand == "American Express" && len(digits) == 15:
		groupSizes = []int{4, 6, 5}
	case brand == "Diners Club" && len(digits) == 14:
		groupSizes = []int{4, 6, 4}
	}
	var groups []string
	for _, size := range groupSizes {
		if digits == "" {
			break
		}
		size = min(size, len(digits))
		groups, digits = append(groups, digits[:size]), digits[size:]
	}
	if digits != "" {
		groups = append(groups, digits)
	}
	return strings.Join(groups, " ")
}

// cardBrands is checked in order; a prefix range is inclusive and compared on as many digits as its bounds have.
var cardBrands = []struct {
	name   string
	ranges [][2]int
}{
	{"Troy", [][2]int{{9792, 9792}}},
	{"American Express", [][2]int{{34, 34}, {37, 37}}},
	{"JCB", [][2]int{{3528, 3589}}},
	{"Diners Club", [][2]int{{300, 305}, {36, 36}, {38, 39}}},
	{"Discover", [][2]int{{6011, 6011}, {644, 649}, {65, 65}}},
	{"UnionPay", [][2]int{{62, 62}}},
	{"Mastercard", [][2]int{{51, 55}, {2221, 2720}}},
	{"Maestro", [][2]int{{50, 50}, {56, 58}, {6304, 6304}, {6759, 6759}, {6761, 6763}}},
	{"Visa", [][2]int{{4, 4}}},
}

// ListCardBrands returns the brands Hisar recognizes, in the order an editor offers them as a card's type.
func ListCardBrands() []string {
	return []string{"Visa", "Mastercard", "American Express", "Discover", "Diners Club", "JCB", "UnionPay", "Maestro", "Troy"}
}

// cardTypeAliases maps other ways of writing a brand, such as 1Password's "mc" or Bitwarden's "Amex", to its name.
var cardTypeAliases = map[string]string{
	"mc":              "Mastercard",
	"master card":     "Mastercard",
	"amex":            "American Express",
	"americanexpress": "American Express",
	"diners":          "Diners Club",
	"dinersclub":      "Diners Club",
	"carteblanche":    "Diners Club",
	"union pay":       "UnionPay",
	"china unionpay":  "UnionPay",
	"visaelectron":    "Visa",
	"visa electron":   "Visa",
}

// NormalizeCardType writes a known brand the way ListCardBrands does, whatever its case or spelling, and keeps any other
// type as typed.
func NormalizeCardType(cardType string) string {
	cardType = strings.TrimSpace(cardType)
	for _, brand := range ListCardBrands() {
		if strings.EqualFold(cardType, brand) {
			return brand
		}
	}
	if brand, isFound := cardTypeAliases[strings.ToLower(cardType)]; isFound {
		return brand
	}
	return cardType
}

// GetCardBrand names the card network a number belongs to by its first digits, or returns "".
func GetCardBrand(number string) string {
	digits := getCardDigits(number)
	for _, brand := range cardBrands {
		for _, prefixRange := range brand.ranges {
			length := len(strconv.Itoa(prefixRange[0]))
			if len(digits) < length {
				continue
			}
			if prefix, err := strconv.Atoi(digits[:length]); err == nil && prefix >= prefixRange[0] && prefix <= prefixRange[1] {
				return brand.name
			}
		}
	}
	return ""
}

// GetCardLast4 returns the last four digits of a card number, or "" when it has fewer.
func GetCardLast4(number string) string {
	digits := getCardDigits(number)
	if len(digits) < 4 {
		return ""
	}
	return digits[len(digits)-4:]
}

// IsValidCardNumber reports whether a number has the length of a card number and passes the Luhn check.
func IsValidCardNumber(number string) bool {
	digits := getCardDigits(number)
	if len(digits) < 12 || len(digits) > 19 {
		return false
	}
	sum := 0
	for index := range len(digits) {
		digit := int(digits[len(digits)-1-index] - '0')
		if index%2 == 1 {
			if digit *= 2; digit > 9 {
				digit -= 9
			}
		}
		sum += digit
	}
	return sum%10 == 0
}

var expiryPatterns = []struct {
	pattern    *regexp.Regexp
	monthGroup int
	yearGroup  int
}{
	{regexp.MustCompile(`^(\d{1,2})\s*[/.-]\s*(\d{2}|\d{4})$`), 1, 2},
	{regexp.MustCompile(`^(\d{4})\s*[/.-]\s*(\d{1,2})$`), 2, 1},
	{regexp.MustCompile(`^(\d{2})(\d{2})$`), 1, 2},
}

// NormalizeExpiry writes an expiry such as 12/27, 1227, 12/2027 or 2027-12 as MM/YY; isValid is false for anything else.
func NormalizeExpiry(expiry string) (normalized string, isValid bool) {
	expiry = strings.TrimSpace(expiry)
	for _, expiryPattern := range expiryPatterns {
		match := expiryPattern.pattern.FindStringSubmatch(expiry)
		if match == nil {
			continue
		}
		month, _ := strconv.Atoi(match[expiryPattern.monthGroup])
		year, _ := strconv.Atoi(match[expiryPattern.yearGroup])
		if month < 1 || month > 12 {
			return "", false
		}
		return fmt.Sprintf("%02d/%02d", month, year%100), true
	}
	return "", false
}

// CardDescription is what an editor shows about the number and expiry being typed; a warning never stops a save.
type CardDescription struct {
	Brand         string
	Last4         string
	NumberWarning string
	ExpiryWarning string
}

// DescribeCard names the brand and last four digits and warns about a number or expiry that looks wrong at now.
func DescribeCard(number, expiry string, now time.Time) CardDescription {
	description := CardDescription{Brand: GetCardBrand(number), Last4: GetCardLast4(number)}
	if strings.TrimSpace(number) != "" && !IsValidCardNumber(number) {
		description.NumberWarning = "This doesn't look like a valid card number"
	}
	if strings.TrimSpace(expiry) != "" {
		normalized, isValid := NormalizeExpiry(expiry)
		switch {
		case !isValid:
			description.ExpiryWarning = "Use month and year, like 12/27"
		case isExpired(normalized, now):
			description.ExpiryWarning = "This card has expired"
		}
	}
	return description
}

// isExpired: a card is valid through the last day of its expiry month, in now's time zone.
func isExpired(normalizedExpiry string, now time.Time) bool {
	month, _ := strconv.Atoi(normalizedExpiry[:2])
	year, _ := strconv.Atoi(normalizedExpiry[3:])
	year += 2000
	return !now.Before(time.Date(year, time.Month(month)+1, 1, 0, 0, 0, 0, now.Location()))
}
