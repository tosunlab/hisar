package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestStreamEvents(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/events" || request.Header.Get("Authorization") != "Bearer token" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(writer, ": connected\n\n")
		fmt.Fprint(writer, "event: vault-changed\ndata: {\"vaultID\":\"v1\",\"seq\":7}\n\n")
		fmt.Fprint(writer, ": heartbeat\n\n")
		fmt.Fprint(writer, "event: vaults-changed\ndata: {}\n\n")
		fmt.Fprint(writer, "data: first\ndata: second\n\n")
	}))
	defer testServer.Close()
	apiClient := Create(testServer.URL)
	apiClient.Token = "token"
	var allEvent []Event
	err := apiClient.StreamEvents(context.Background(), func(event Event) { allEvent = append(allEvent, event) })
	if err != nil {
		t.Fatalf("stream ended with %v", err)
	}
	if len(allEvent) != 3 || allEvent[0].Name != "vault-changed" || string(allEvent[0].Data) != `{"vaultID":"v1","seq":7}` ||
		allEvent[1].Name != "vaults-changed" || string(allEvent[2].Data) != "first\nsecond" {
		t.Errorf("events = %+v", allEvent)
	}

	apiClient.Token = "wrong"
	if err := apiClient.StreamEvents(context.Background(), func(Event) {}); !IsStatus(err, http.StatusUnauthorized) {
		t.Errorf("unauthorized stream: %v", err)
	}
}

func TestStreamEventsOutlivesRequestTimeoutAndStopsWithContext(t *testing.T) {
	var waitGroup sync.WaitGroup
	waitGroup.Add(1)
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer waitGroup.Done()
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.(http.Flusher).Flush()
		for {
			select {
			case <-request.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
				fmt.Fprint(writer, ": heartbeat\n\n")
				writer.(http.Flusher).Flush()
			}
		}
	}))
	defer testServer.Close()
	apiClient := Create(testServer.URL)
	apiClient.HTTP.Timeout = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	err := apiClient.StreamEvents(ctx, func(Event) {})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(startedAt) < 250*time.Millisecond {
		t.Errorf("stream ended after %v with %v; the request timeout must not apply", time.Since(startedAt), err)
	}
	waitGroup.Wait()
}
