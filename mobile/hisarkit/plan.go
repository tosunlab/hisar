package hisarkit

// GetPlan returns the account's plan (vault.Plan) as JSON. The iPhone only shows it: it never starts a checkout.
func (hisar *Hisar) GetPlan() (string, error) {
	return encodeJSON(hisar.service.GetPlan())
}
