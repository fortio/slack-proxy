// server.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"fortio.org/log"
)

func (app *App) StartServer(ctx context.Context, applicationPort *string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handleRequest)

	server := &http.Server{
		Addr:    *applicationPort,
		Handler: mux,
	}

	doneCh := make(chan error)
	go func() {
		// Start the server
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			doneCh <- err
		}
		close(doneCh)
	}()

	select {
	case <-ctx.Done():
		return server.Shutdown(ctx)
	case err := <-doneCh:
		return err
	}
}

func (app *App) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Regardless of the outcome, we always respond as json
	w.Header().Set("Content-Type", "application/json")

	// "Mock" the response from Slack.
	// OK is true by default, so we only need to set it to false if we want to trow an error which then could use a custom error message.
	// From testing, any application only checks if OK is true. So we can ignore all other fields
	fakeSlackResponse := SlackResponse{
		Ok: true,
	}

	maxQueueSize := int(float64(cap(app.slackQueue)) * 0.9)
	// Reject requests if the queue is almost full
	// If the channel is full, the request will block until there is space in the channel.
	// Ideally we don't reject at 90%, but initially after some tests I got blocked. So I decided to be a bit more conservative.
	// ToDo: Fix this behavior so we can reach 100% channel size without problems.
	if len(app.slackQueue) >= maxQueueSize {
		w.WriteHeader(http.StatusServiceUnavailable)

		fakeSlackResponse.Ok = false
		fakeSlackResponse.Error = "Queue is almost full"
		responseData, err := json.Marshal(fakeSlackResponse)
		if err != nil {
			http.Error(w, "Failed to serialize Slack response", http.StatusInternalServerError)
			return
		}

		_, err = w.Write(responseData)
		if err != nil {
			log.S(log.Error, "Failed to write response", log.Any("err", err))
		}

		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request SlackPostMessageRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, "Failed to read the request body", http.StatusInternalServerError)
		return
	}

	// Validate the request
	err = validate(request)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fakeSlackResponse.Ok = false
		fakeSlackResponse.Error = err.Error()
		responseData, err := json.Marshal(fakeSlackResponse)
		log.S(log.Error, "Invalid request", log.Any("err", err))
		_, err = w.Write(responseData)
		if err != nil {
			log.S(log.Error, "Failed to write response", log.Any("err", err))
		}
		return
	}

	app.metrics.RequestsReceivedTotal.WithLabelValues(request.Channel).Inc()

	responseData, err := json.Marshal(fakeSlackResponse)
	if err != nil {
		http.Error(w, "Failed to serialize Slack response", http.StatusInternalServerError)
		return
	}

	// Add a counter to the wait group, this is important to wait for all the messages to be processed before shutting down the server.
	app.wg.Add(1)
	// Send the message to the slackQueue to be processed
	app.slackQueue <- request
	// Update the queue size metric after any change on the queue size
	app.metrics.QueueSize.With(nil).Set(float64(len(app.slackQueue)))

	// Respond, this is not entirely accurate as we have no idea if the message will be processed successfully.
	// This is the downside of having a queue which could potentially delay responses by a lot.
	// We do our due diligences on the received message and can make a fair assumption we will be able to process it.
	// Application should utlise this applications metrics and logs to find out if there are any issues.
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(responseData)
	if err != nil {
		log.S(log.Error, "Failed to write response", log.Any("err", err))
	}
}

func validate(request SlackPostMessageRequest) error {
	var errorMessages []string

	// Check if 'Channel' is set
	if request.Channel == "" {
		errorMessages = append(errorMessages, "Channel is not set")
	}

	// Check if at least one of 'attachments', 'blocks', or 'text' is set
	if len(request.Attachments) == 0 && len(request.Blocks) == 0 && request.Text == "" {
		errorMessages = append(errorMessages, "Neither attachments, blocks, nor text is set")
	}

	if len(errorMessages) > 0 {
		return fmt.Errorf(strings.Join(errorMessages, " and "))
	}

	return nil
}
