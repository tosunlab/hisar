package browserprotocol

import (
	"context"
	"encoding/json"
	"errors"
)

// Handler answers one request; returning a *ResponseError sends its code to the extension.
type Handler interface {
	HandleRequest(ctx context.Context, request *Request) (any, error)
}

// Answer decodes one request message and answers it with handler; an error that is not a *ResponseError
// is reported as ErrorInternal.
func Answer(ctx context.Context, handler Handler, message []byte) Response {
	var request Request
	if err := json.Unmarshal(message, &request); err != nil {
		return Response{ID: request.ID, Error: NewError(ErrorInvalidRequest, "the request is not valid")}
	}
	return AnswerRequest(ctx, handler, &request)
}

// AnswerRequest answers a decoded request, preserving the wire error contract.
func AnswerRequest(ctx context.Context, handler Handler, request *Request) Response {
	if request.ID == "" || request.Type == "" {
		return Response{ID: request.ID, Error: NewError(ErrorInvalidRequest, "the request is not valid")}
	}
	result, err := handler.HandleRequest(ctx, request)
	if err != nil {
		var responseError *ResponseError
		if !errors.As(err, &responseError) {
			responseError = NewError(ErrorInternal, err.Error())
		}
		return Response{ID: request.ID, Error: responseError}
	}
	if result == nil {
		result = struct{}{}
	}
	return Response{ID: request.ID, OK: true, Result: result}
}

// BrowserInstallation is the native host manifest of one installed browser, as Settings shows it.
type BrowserInstallation struct {
	Name         string `json:"name"`
	ManifestPath string `json:"manifestPath"`
	IsInstalled  bool   `json:"isInstalled"`
	ErrorMessage string `json:"errorMessage"`
}
