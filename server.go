// server.go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/jrpc"
	"fortio.org/log"
)

func (app *App) StartServer(ctx context.Context, applicationPort string) error {
	// TODO probably switch to fhttp but need to see if I can add a shutdown hook.
	name := "tbd" // TODO: Add a name field to "App"
	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handleRequest)

	server := &http.Server{
		Addr:              applicationPort,
		Handler:           mux,
		ReadHeaderTimeout: fhttp.ServerIdleTimeout.Get(),
		IdleTimeout:       fhttp.ServerIdleTimeout.Get(),
		ErrorLog:          log.NewStdLogger("http srv "+name, log.Error),
	}

	doneCh := make(chan error)
	go func() {
		// Start the server
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
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
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	maxQueueSize := int(float64(cap(app.slackQueue)) * 0.9)
	// Reject requests if the queue is almost full
	// If the channel is full, the request will block until there is space in the channel.
	// Ideally we don't reject at 90%, but initially after some tests I got blocked. So I decided to be a bit more conservative.
	// ToDo: Fix this behavior so we can reach 100% channel size without problems.
	if len(app.slackQueue) >= maxQueueSize {
		log.S(log.Warning, "Queue is almost full, returning StatusServiceUnavailable", log.Int("queueSize", len(app.slackQueue)))

		err := jrpc.Reply[SlackResponse](w, http.StatusServiceUnavailable, &SlackResponse{
			Ok:    false,
			Error: "Queue is almost full",
		})
		if err != nil {
			log.S(log.Error, "Failed to write response", log.Any("err", err))
		}

		return
	}

	var request SlackPostMessageRequest
	requestErr := json.NewDecoder(r.Body).Decode(&request)

	// If we can't decode, we don't bother validating. In the end it's the same outcome if either one is invalid.
	if requestErr == nil {
		requestErr = validate(request)
	}

	if requestErr != nil {
		log.S(log.Error, "Invalid request", log.Any("err", requestErr))

		err := jrpc.Reply[SlackResponse](w, http.StatusBadRequest, &SlackResponse{
			Ok:    false,
			Error: requestErr.Error(),
		})
		if err != nil {
			log.S(log.Error, "Failed to write response", log.Any("err", err))
		}
		return
	}

	// Start the logic (as we passed all our checks) to process the request.
	app.metrics.RequestsReceivedTotal.WithLabelValues(request.Channel).Inc()

	// If the channelOverride flag is set, we override the channel for all messages.
	// We still use the original channel for the metrics (see above).
	if app.channelOverride != "" {
		log.S(log.Debug, "Overriding channel", log.String("channelOverride", app.channelOverride), log.String("channel", request.Channel))
		request.Channel = app.channelOverride
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
	err := jrpc.Reply[SlackResponse](w, http.StatusOK, &SlackResponse{
		Ok: true,
	})
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
		return fmt.Errorf("%s", strings.Join(errorMessages, " and "))
	}

	return nil
}
