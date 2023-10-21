// server.go
package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
)

func (app *App) StartServer(ctx context.Context, applicationPort *string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

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
		// Ideally we don't reject at 90%, but initialy after some tests I got blocked. So I decided to be a bit more conservative.
		// ToDo: Fix this behaviour so we can reach 100% channel size without problems.
		if len(app.slackQueue) >= maxQueueSize {
			w.WriteHeader(http.StatusServiceUnavailable)

			fakeSlackResponse.Ok = false
			fakeSlackResponse.Error = "Queue is almost full"
			responseData, err := json.Marshal(fakeSlackResponse)
			if err != nil {
				http.Error(w, "Failed to serialize Slack response", http.StatusInternalServerError)
				return
			}

			w.Write(responseData)

			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read the request body", http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()

		var request SlackPostMessageRequest
		err = json.Unmarshal(body, &request)
		if err != nil {
			http.Error(w, "Failed to parse the request body", http.StatusBadRequest)
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
		// We do our due diligences on the recieved message and can make a fair assumption we will be able to process it.
		// Application should utlise this applications metrics and logs to find out if there are any issues.
		w.WriteHeader(http.StatusOK)
		w.Write(responseData)

	})

	server := &http.Server{Addr: *applicationPort}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			panic(err)
		}
	}()

	<-ctx.Done() // wait for context cancellation
	server.Shutdown(ctx)
}
