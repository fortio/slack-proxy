// server.go
package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
)

func (app *App) StartServer(ctx context.Context) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Content-Type", "application/json")

		fakeSlackResponse := SlackResponse{
			Ok: true,
		}

		maxQueueSize := int(float64(cap(app.slackQueue)) * 0.9)
		// Reject requests if the queue is almost full
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

		responseData, err := json.Marshal(fakeSlackResponse)
		if err != nil {
			http.Error(w, "Failed to serialize Slack response", http.StatusInternalServerError)
			return
		}

		// Respond, this is not entirely accurate as we have no idea if the message will be processed successfully.
		// This is the downside of having a queue which could potentially delay responses by a lot.
		// We do our due diligences on the recieved message and can make a fair assumption we will be able to process it.
		// Application should utlise this applications metrics and logs to find out if there are any issues.
		w.WriteHeader(http.StatusOK)
		w.Write(responseData)

		app.wg.Add(1)
		// Send the message to the slackQueue to be processed
		app.slackQueue <- request
		// Add a counter to the wait group

	})

	server := &http.Server{Addr: ":8080"}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			panic(err)
		}
	}()

	<-ctx.Done() // wait for context cancellation
	server.Shutdown(ctx)
}
