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

const updateWriteIdleTimeout = 60 * time.Second

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

// deadlineExtendingReader moves the write deadline with every read, so a slow download is not cut off by WriteTimeout.
type deadlineExtendingReader struct {
	reader     io.ReadCloser
	connection net.Conn
}

func (streamed *deadlineExtendingReader) Read(buffer []byte) (int, error) {
	streamed.connection.SetWriteDeadline(time.Now().Add(updateWriteIdleTimeout))
	return streamed.reader.Read(buffer)
}

func (streamed *deadlineExtendingReader) Close() error { return streamed.reader.Close() }

func (s *Server) handleDownloadDesktopUpdate(c fiber.Ctx) error {
	if s.config.DesktopUpdates == nil {
		return errUpdatesNotConfigured
	}
	version, fileName := c.Params("version"), c.Params("file")
	if !desktopupdate.IsValidVersion(version) || !desktopupdate.IsValidFileName(fileName) {
		return errNotFound
	}
	// The body is streamed after the handler returns, so the request must outlive this context.
	reader, size, err := s.config.DesktopUpdates.OpenFile(context.Background(), version, fileName)
	if errors.Is(err, desktopupdate.ErrFileNotFound) {
		return errNotFound
	}
	if err != nil {
		log.Printf("desktop update %s/%s: %v", version, fileName, err)
		return createAPIError(fiber.StatusBadGateway, "the update source is unreachable")
	}
	c.Set(fiber.HeaderContentType, "application/octet-stream")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="`+fileName+`"`)
	return c.SendStream(&deadlineExtendingReader{reader: reader, connection: c.RequestCtx().Conn()}, int(size))
}
