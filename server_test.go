// server_test.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fortio.org/assert"
	"github.com/prometheus/client_golang/prometheus"
)

func TestHandleRequest(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		body       string
		wantStatus int
		wantBody   SlackResponse
	}{
		{
			name:       "valid post request",
			method:     http.MethodPost,
			body:       `{"channel": "test_channel", "text": "Hello"}`,
			wantStatus: http.StatusOK,
			wantBody:   SlackResponse{Ok: true},
		},
		{
			name:       "invalid method",
			method:     http.MethodGet,
			body:       ``,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "invalid post request",
			method:     http.MethodPost,
			body:       `{"foo": "bar"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   SlackResponse{Ok: false, Error: "Channel is not set and Neither attachments, blocks, nor text is set"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := prometheus.NewRegistry()
			metrics := NewMetrics(r)

			app := &App{
				slackQueue: make(chan SlackPostMessageRequest, 10),
				metrics:    metrics,
			}

			req, err := http.NewRequest(tt.method, "/", bytes.NewBufferString(tt.body))
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			app.handleRequest(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)

			if tt.wantBody != (SlackResponse{}) {
				var response SlackResponse
				err := json.NewDecoder(rr.Body).Decode(&response)
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, tt.wantBody, response)
			}
		})
	}
}

func TestStartServer(t *testing.T) {
	r := prometheus.NewRegistry()
	metrics := NewMetrics(r)
	app := &App{
		slackQueue: make(chan SlackPostMessageRequest, 10),
		metrics:    metrics,
	}
	testPort := ":9090"
	ctx, cancel := context.WithCancel(context.Background())

	// T.Fatalf, which must be called in the same goroutine as the test (SA2002)
	// Sue me, I don't know how to fix this nicer than this...
	errCh := make(chan error)
	go func() {
		if err := app.StartServer(ctx, testPort); err != nil && err != http.ErrServerClosed {
			errCh <- err // Send the error to the channel
		}
		close(errCh)
	}()

	// Give server some time to start
	// If you are running on a non-priviledged account, and get a popup asking for permission to accept incoming connections, you can increase this time...
	time.Sleep(1 * time.Second)

	// Make a sample request to ensure server is running

	resp, err := http.Post("http://localhost"+testPort, "application/json", bytes.NewBufferString(`{"channel": "test_channel", "text": "Hello"}`))
	if err != nil {
		t.Fatalf("Could not make GET request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status OK, got: %v", resp.StatusCode)
	}

	// Cancel the context, which should stop the server
	cancel()

	// Give server some time to shut down
	time.Sleep(1 * time.Second)

	// Make another request, this should fail since the server should be stopped
	_, err = http.Get("http://localhost" + testPort)
	if err == nil {
		t.Fatal("Expected error making GET request after server shut down, got none")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Error starting server: %v", err)
		}
	default:
		// No error received from the channel
	}
}
