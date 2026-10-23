package item

import (
	"fmt"
	"testing"
)

func TestNormalizeKeepsStructuredSecrets(t *testing.T) {
	for _, secret := range []string{" ", "\u00a0", "\t ", " secret ", "\u00a0secret\u00a0"} {
		for _, test := range []struct {
			category  string
			names     []string
			normalize func(*Item)
		}{
			{CategoryAPICredential, []string{FieldSecret}, NormalizeAPICredential},
			{CategoryCreditCard, []string{FieldSecurityCode, FieldPIN}, NormalizeCard},
			{CategoryIdentity, []string{FieldPassportNumber, FieldDriversLicenseNumber, FieldNationalIDNumber}, NormalizeIdentity},
			{CategorySoftwareLicense, []string{FieldLicenseKey}, NormalizeSoftwareLicense},
		} {
			t.Run(fmt.Sprintf("%s/%q", test.category, secret), func(t *testing.T) {
				vaultItem := Item{Overview: Overview{Category: test.category}}
				for _, name := range test.names {
					vaultItem.Details.Fields = append(vaultItem.Details.Fields, Field{Name: name, Value: secret})
				}
				test.normalize(&vaultItem)
				if len(vaultItem.Details.Fields) != len(test.names) {
					t.Fatalf("secret fields = %+v", vaultItem.Details.Fields)
				}
				for _, field := range vaultItem.Details.Fields {
					if field.Value != secret || !field.Concealed {
						t.Errorf("%s = %q, concealed %v; want %q concealed", field.Name, field.Value, field.Concealed, secret)
					}
				}
			})
		}
	}
}
