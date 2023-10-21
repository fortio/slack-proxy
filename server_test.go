// server_test.go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
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

			logger, err := zap.NewDevelopment()
			if err != nil {
				panic("failed to initialize logger: " + err.Error())
			}
			r := prometheus.NewRegistry()
			metrics := NewMetrics(r)

			app := &App{
				logger:     logger,
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
