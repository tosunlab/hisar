package browser

// PasskeyPRFValues holds base64url PRF inputs or outputs; Second is nil when not given.
type PasskeyPRFValues struct {
	First  string  `json:"first"`
	Second *string `json:"second,omitempty"`
}

type PasskeyPRFRequest struct {
	Eval             *PasskeyPRFValues           `json:"eval,omitempty"`
	EvalByCredential map[string]PasskeyPRFValues `json:"evalByCredential,omitempty"`
}

type PasskeyPRFResult struct {
	IsEnabled bool              `json:"enabled,omitempty"`
	Results   *PasskeyPRFValues `json:"results,omitempty"`
}

type PasskeySignalResult struct {
	ChangedPasskeyCount int `json:"changedPasskeyCount"`
}
