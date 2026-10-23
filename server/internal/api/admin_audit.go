package api

import (
	"bufio"
	"context"
	"encoding/csv"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/service"
)

// auditExportWriteTimeout bounds writing one batch of an export; the whole export may outlast WriteTimeout.
const auditExportWriteTimeout = time.Minute

var auditExportHeader = []string{"ID", "Time", "Event", "Type", "Account ID", "Account email", "Device ID", "IP address", "Details"}

func (s *Server) registerAdminAuditRoutes(adminGroup fiber.Router) {
	adminGroup.Get("/audit", s.requireSession, requireAdmin, s.handleListAuditEvents)
	adminGroup.Get("/audit/types", s.requireSession, requireAdmin, s.handleGetAuditTypes)
	adminGroup.Get("/audit.csv", s.requireSession, requireAdmin, s.handleExportAuditEvents)
}

func getAuditQuery(c fiber.Ctx) service.AuditQuery {
	return service.AuditQuery{
		Account: c.Query("account"), Type: c.Query("type"), Group: c.Query("group"), From: c.Query("from"), To: c.Query("to"), Cursor: c.Query("cursor"),
	}
}

func (s *Server) handleListAuditEvents(c fiber.Ctx) error {
	page, err := s.service.ListAuditEvents(c.Context(), getAuditQuery(c))
	if err != nil {
		return err
	}
	return c.JSON(page)
}

func (s *Server) handleGetAuditTypes(c fiber.Ctx) error {
	return c.JSON(s.service.GetAuditTypes())
}

func (s *Server) handleExportAuditEvents(c fiber.Ctx) error {
	export, err := s.service.StartAuditExport(c.Context(), s.getActor(c), getAuditQuery(c))
	if err != nil {
		return err
	}
	batch, err := export.Next(c.Context())
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="hisar-audit-`+time.Now().UTC().Format(time.DateOnly)+`.csv"`)
	c.Set(fiber.HeaderCacheControl, "no-store")
	connection := c.RequestCtx().Conn()
	return c.SendStreamWriter(func(writer *bufio.Writer) {
		csvWriter := csv.NewWriter(writer)
		csvWriter.Write(auditExportHeader)
		rowCount := 0
		for {
			connection.SetWriteDeadline(time.Now().Add(auditExportWriteTimeout))
			for _, event := range batch {
				csvWriter.Write(formatAuditRow(event))
			}
			rowCount += len(batch)
			csvWriter.Flush()
			if csvWriter.Error() != nil || writer.Flush() != nil || len(batch) == 0 {
				return
			}
			// The body is written after the handler returns, so each batch gets its own context.
			batchContext, cancel := context.WithTimeout(context.Background(), s.config.RequestTimeout)
			batch, err = export.Next(batchContext)
			cancel()
			if err != nil {
				log.Printf("audit export after %d rows: %v", rowCount, err)
				// Closing the connection before the last chunk tells the browser the file is incomplete.
				connection.Close()
				return
			}
		}
	})
}

func formatAuditRow(event apitypes.AuditEvent) []string {
	row := []string{
		strconv.FormatInt(event.ID, 10), event.CreatedAt.UTC().Format(time.RFC3339Nano), service.LabelAuditType(event.Type), event.Type,
		dereference(event.AccountID), dereference(event.AccountEmail), dereference(event.DeviceID), event.IP, string(event.Detail),
	}
	for index, field := range row {
		if field != "" && strings.ContainsRune("=+-@\t\r", rune(field[0])) {
			row[index] = "'" + field
		}
	}
	return row
}

func dereference(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
