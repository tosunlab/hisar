package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStartCallRetriesOnlyOneDocumentedPreEntryRefusal(t *testing.T) {
	missing := signInAnswer{IsError: true, Text: "The window shows a page that is not a website. Open the site's sign-in page and call sign_in again."}
	blank := signInAnswer{IsError: true, Text: "The window shows about:blank, not a website. Open the site's sign-in page and call sign_in again."}
	for _, test := range []struct {
		name    string
		answers []signInAnswer
		retries int64
	}{
		{"missing page then submitted", []signInAnswer{missing, {Outcome: "submitted"}}, 1},
		{"blank page still blank", []signInAnswer{blank, blank}, 1},
		{"cleanup unverified", []signInAnswer{{Outcome: "cleanup-failed", Cleanup: "unverified"}}, 0},
		{"password rejected", []signInAnswer{{Outcome: "not-accepted"}}, 0},
		{"origin refused", []signInAnswer{{IsError: true, Text: "No login is saved for this origin."}}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
			var calls atomic.Int64
			type input struct {
				PID      int    `json:"pid"`
				WindowID uint32 `json:"window_id"`
				Login    string `json:"login"`
			}
			mcp.AddTool(server, &mcp.Tool{Name: "sign_in"}, func(_ context.Context, _ *mcp.CallToolRequest, got input) (*mcp.CallToolResult, signInAnswer, error) {
				if got.PID != 123 || got.WindowID != 456 || got.Login != "login-id" {
					t.Errorf("sign_in arguments = %+v", got)
				}
				index := int(calls.Add(1)) - 1
				if index >= len(test.answers) {
					return nil, signInAnswer{}, errors.New("unexpected extra sign_in call")
				}
				answer := test.answers[index]
				result := &mcp.CallToolResult{IsError: answer.IsError}
				if answer.Text != "" {
					result.Content = []mcp.Content{&mcp.TextContent{Text: answer.Text}}
				}
				return result, answer, nil
			})
			serverTransport, clientTransport := mcp.NewInMemoryTransports()
			serverSession, err := server.Connect(ctx, serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer serverSession.Close()
			session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			current := &run{}
			client := &agent{session: session, transcript: &lockedBuffer{}}
			answer, err := current.startCall(client, cuaWindow{PID: 123, WindowID: 456}, "login-id").wait()
			if err != nil {
				t.Fatal(err)
			}
			want := test.answers[len(test.answers)-1]
			if answer.Outcome != want.Outcome || answer.IsError != want.IsError || (want.IsError && answer.Text != want.Text) {
				t.Fatalf("answer = %+v, want %+v", answer, want)
			}
			if calls.Load() != int64(len(test.answers)) || current.retries.Load() != test.retries {
				t.Fatalf("calls=%d retries=%d, want calls=%d retries=%d", calls.Load(), current.retries.Load(), len(test.answers), test.retries)
			}
		})
	}
}

func TestPageRefusalIsNotRetriedAfterATransportError(t *testing.T) {
	answer := signInAnswer{IsError: true, Text: "The window shows about:blank, not a website. Open the site's sign-in page and call sign_in again."}
	if canRetryPageRefusal(answer, errors.New("connection closed")) {
		t.Fatal("a transport error was retryable")
	}
}
