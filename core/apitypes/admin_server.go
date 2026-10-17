package apitypes

// AdminServer is GET /api/v1/admin/server: what the admin dashboard's Overview shows about the server.
type AdminServer struct {
	PublicURL string `json:"publicUrl"`
}
