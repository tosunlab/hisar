package item

import "slices"

// CategorySoftwareLicense items keep a software license in custom fields with the names below, as API credentials do, so
// an app that does not know them shows them as fields and keeps them when it edits the item. A hisar:// reference names a
// field by its name, such as hisar://Work/Sketch/License key.
const CategorySoftwareLicense = "software-license"

// The custom field names of a software license's values; the licensed-to email is FieldEmail, as an identity's.
const (
	FieldProduct      = "Product"
	FieldVersion      = "Version"
	FieldLicenseKey   = "License key"
	FieldLicensedTo   = "Licensed to"
	FieldPurchaseDate = "Purchase date"
	FieldDownloadURL  = "Download URL"
)

// SoftwareLicense is what a software license item holds. DownloadURL is a field, not the item's website, so the extension
// and AutoFill never offer the license on that site.
type SoftwareLicense struct {
	Product      string
	Version      string
	LicenseKey   string
	LicensedTo   string
	Email        string
	PurchaseDate string
	DownloadURL  string
}

// SoftwareLicenseField is one of a software license's values.
type SoftwareLicenseField = NamedField[SoftwareLicense]

// ListSoftwareLicenseFields returns a software license's fields in the order they are stored and asked for.
func ListSoftwareLicenseFields() []SoftwareLicenseField { return slices.Clone(softwareLicenseFields) }

// softwareLicenseFields is the order of a software license's fields; the license key is concealed.
var softwareLicenseFields = []SoftwareLicenseField{
	{FieldProduct, false, func(license *SoftwareLicense) *string { return &license.Product }},
	{FieldVersion, false, func(license *SoftwareLicense) *string { return &license.Version }},
	{FieldLicenseKey, true, func(license *SoftwareLicense) *string { return &license.LicenseKey }},
	{FieldLicensedTo, false, func(license *SoftwareLicense) *string { return &license.LicensedTo }},
	{FieldEmail, false, func(license *SoftwareLicense) *string { return &license.Email }},
	{FieldPurchaseDate, false, func(license *SoftwareLicense) *string { return &license.PurchaseDate }},
	{FieldDownloadURL, false, func(license *SoftwareLicense) *string { return &license.DownloadURL }},
}

// ReadSoftwareLicense returns the license's values and the item's other custom fields, matching names ignoring case.
func ReadSoftwareLicense(vaultItem *Item) (SoftwareLicense, []Field) {
	return readRecord(vaultItem, softwareLicenseFields)
}

// WriteSoftwareLicense stores license as the item's first custom fields, in a fixed order with the license key concealed,
// followed by otherFields; an empty value is left out.
func WriteSoftwareLicense(vaultItem *Item, license SoftwareLicense, otherFields []Field) {
	writeRecord(vaultItem, license, softwareLicenseFields, otherFields)
}

// NormalizeSoftwareLicense trims a software license item's display values, keeps its license key exact, writes its
// purchase date as YYYY-MM-DD when it is given year first and puts its fields in order; other items are left alone.
// A download URL keeps its text: the apps add https:// only to open it.
func NormalizeSoftwareLicense(vaultItem *Item) {
	if !vaultItem.IsCategory(CategorySoftwareLicense) {
		return
	}
	license, otherFields := ReadSoftwareLicense(vaultItem)
	trimRecord(&license, softwareLicenseFields)
	license.PurchaseDate = NormalizeDate(license.PurchaseDate)
	WriteSoftwareLicense(vaultItem, license, otherFields)
}
