package api

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/server/internal/desktopupdate"
)

const updateIdleTimeout = 60 * time.Second

var errUpdatesNotConfigured = createAPIError(fiber.StatusNotFound, "updates are not configured on this server")

func (s *Server) limitUpdateRate(c fiber.Ctx) error {
	if !s.updateLimiter.Allow(s.getClientIP(c)) {
		return createAPIError(fiber.StatusTooManyRequests, "too many requests")
	}
	return c.Next()
}

func (s *Server) handleGetDesktopAppcast(c fiber.Ctx) error {
	if s.config.DesktopUpdates == nil {
		return errUpdatesNotConfigured
	}
	appcast, err := s.config.DesktopUpdates.GetAppcast(context.Background())
	if err != nil {
		log.Printf("desktop updates: %v", err)
		return createAPIError(fiber.StatusBadGateway, "the update source is unreachable")
	}
	c.Set(fiber.HeaderContentType, "application/xml; charset=utf-8")
	return c.Send(appcast)
}

// deadlineExtendingReader moves the write deadline with every read, so a slow download is not cut off by WriteTimeout,
// and cancels the upstream download when one read waits longer than idleTimeout, so a stalled source cannot hold the
// connection open.
type deadlineExtendingReader struct {
	reader      io.ReadCloser
	connection  net.Conn
	idleTimeout time.Duration
	idleTimer   *time.Timer
	cancel      context.CancelFunc
}

func newDeadlineExtendingReader(reader io.ReadCloser, cancel context.CancelFunc, connection net.Conn, idleTimeout time.Duration) *deadlineExtendingReader {
	return &deadlineExtendingReader{reader: reader, connection: connection, idleTimeout: idleTimeout, idleTimer: time.AfterFunc(idleTimeout, cancel), cancel: cancel}
}

func (streamed *deadlineExtendingReader) Read(buffer []byte) (int, error) {
	streamed.idleTimer.Reset(streamed.idleTimeout)
	streamed.connection.SetWriteDeadline(time.Now().Add(streamed.idleTimeout))
	return streamed.reader.Read(buffer)
}

func (streamed *deadlineExtendingReader) Close() error {
	streamed.idleTimer.Stop()
	streamed.cancel()
	return streamed.reader.Close()
}

func (s *Server) handleDownloadDesktopUpdate(c fiber.Ctx) error {
	if s.config.DesktopUpdates == nil {
		return errUpdatesNotConfigured
	}
	version, fileName := c.Params("version"), c.Params("file")
	if !desktopupdate.IsValidVersion(version) || !desktopupdate.IsValidFileName(fileName) {
		return errNotFound
	}
	// The body is streamed after the handler returns, so the download must outlive the request's context.
	ctx, cancel := context.WithCancel(context.Background())
	reader, size, err := s.config.DesktopUpdates.OpenFile(ctx, version, fileName)
	switch {
	case errors.Is(err, desktopupdate.ErrFileNotFound):
		cancel()
		return errNotFound
	case err != nil:
		cancel()
		log.Printf("desktop update %s/%s: %v", version, fileName, err)
		return createAPIError(fiber.StatusBadGateway, "the update source is unreachable")
	}
	c.Set(fiber.HeaderContentType, "application/octet-stream")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="`+fileName+`"`)
	return c.SendStream(newDeadlineExtendingReader(reader, cancel, c.RequestCtx().Conn(), updateIdleTimeout), int(size))
}
