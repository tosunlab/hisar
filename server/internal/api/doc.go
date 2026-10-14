// Package api is the HTTP layer of the server: routes, sessions, per-IP rate limits, the event stream and the web
// pages. Each handler parses the request and hands it to internal/service, which decides; refusals are answered
// with the status of their service.Kind.
package api
