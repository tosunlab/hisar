package browserprotocol

import "context"

// Handler answers one request; returning a *ResponseError sends its code to the extension.
type Handler interface {
	HandleRequest(ctx context.Context, request *Request) (any, error)
}

// BrowserInstallation is the native host manifest of one installed browser, as Settings shows it.
type BrowserInstallation struct {
	Name         string `json:"name"`
	ManifestPath string `json:"manifestPath"`
	IsInstalled  bool   `json:"isInstalled"`
	ErrorMessage string `json:"errorMessage"`
}
