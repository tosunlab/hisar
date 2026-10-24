package browserprotocol

const FeaturePasskeyVerification = "passkey-verification-v2"
const ErrorPasskeyVerification = "passkey-verification-failed"

// PasskeyRequest is the nested "passkey" parameter of the passkeys.* requests; binary values are base64url.
type PasskeyRequest struct {
	UserVerification      string   `json:"userVerification,omitempty"`
	CeremonyID            string   `json:"ceremonyId,omitempty"`
	Origin                string   `json:"origin"`
	TopOrigin             string   `json:"topOrigin,omitempty"`
	IsCrossOrigin         bool     `json:"isCrossOrigin,omitempty"`
	RelyingPartyID        string   `json:"relyingPartyId,omitempty"`
	Challenge             string   `json:"challenge,omitempty"`
	UserHandle            string   `json:"userHandle,omitempty"`
	UserName              string   `json:"userName,omitempty"`
	UserDisplayName       string   `json:"userDisplayName,omitempty"`
	AllowedCredentialIDs  []string `json:"allowCredentials,omitempty"`
	ExcludedCredentialIDs []string `json:"excludeCredentials,omitempty"`
	CredentialID          string   `json:"credentialId,omitempty"`
	VaultID               string   `json:"vaultID,omitempty"`
	ItemID                string   `json:"itemID,omitempty"`

	PRF                   *PasskeyPRFRequest `json:"prf,omitempty"`
	AcceptedCredentialIDs []string           `json:"allAcceptedCredentialIds,omitempty"`
}

type PasskeySummary struct {
	CredentialID    string `json:"credentialId"`
	VaultID         string `json:"vaultID"`
	ItemID          string `json:"itemID"`
	Title           string `json:"title"`
	UserName        string `json:"userName"`
	UserDisplayName string `json:"userDisplayName"`
}

type PasskeyListResult struct {
	Passkeys []PasskeySummary `json:"passkeys"`
}

type PasskeyPrepareResult struct {
	IsExcluded bool    `json:"isExcluded"`
	Logins     []Login `json:"logins"`
}

type PasskeyCreationResult struct {
	CredentialID       string `json:"credentialId"`
	ClientDataJSON     string `json:"clientDataJSON"`
	AttestationObject  string `json:"attestationObject"`
	AuthenticatorData  string `json:"authenticatorData"`
	PublicKey          string `json:"publicKey"`
	PublicKeyAlgorithm int    `json:"publicKeyAlgorithm"`
	VaultID            string `json:"vaultID"`
	ItemID             string `json:"itemID"`

	PRF *PasskeyPRFResult `json:"prf,omitempty"`
}

type PasskeyAssertionResult struct {
	CredentialID      string `json:"credentialId"`
	ClientDataJSON    string `json:"clientDataJSON"`
	AuthenticatorData string `json:"authenticatorData"`
	Signature         string `json:"signature"`
	UserHandle        string `json:"userHandle"`

	PRF *PasskeyPRFResult `json:"prf,omitempty"`
}
