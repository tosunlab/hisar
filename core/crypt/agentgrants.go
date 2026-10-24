package crypt

const agentGrantsLabel = "hisar/v1/agent-grants"

// SealAgentGrants seals what this device lets coding agents do without asking with K_user, bound to the account ID: the
// device state is a file any program of the user can write, and only the unlocked account can seal a grant Hisar opens.
func SealAgentGrants(userKey []byte, accountID string, plaintext []byte) ([]byte, error) {
	return Seal(userKey, plaintext, encodeContext(agentGrantsLabel, accountID))
}

func OpenAgentGrants(userKey []byte, accountID string, ciphertext []byte) ([]byte, error) {
	return Open(userKey, ciphertext, encodeContext(agentGrantsLabel, accountID))
}
