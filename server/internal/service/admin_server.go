package service

import "github.com/tryhisar/hisar/core/apitypes"

// GetAdminServer is the server's Overview for the admin dashboard; internal/api lets only admins reach it.
func (s *Service) GetAdminServer() apitypes.AdminServer {
	return apitypes.AdminServer{PublicURL: s.config.PublicURL}
}
