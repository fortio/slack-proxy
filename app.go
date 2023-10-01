// app.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type SlackMessenger interface {
	PostMessage(req SlackPostMessageRequest, url string, token string) error
}

type SlackClient struct {
	client *http.Client
}

func (s *SlackClient) PostMessage(request SlackPostMessageRequest, url string, token string) error {
	jsonValue, err := json.Marshal(request)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonValue))
	if err != nil {
		return err
	}

	// Charset is required to remove warnings from Slack. Maybe it's nice to have it configurable. /shrug
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	// Documentation says that you are allowed the POST the token instead, however that does simply not work. Hence why we are using the Authorization header.
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var slackResp SlackResponse
	if err = json.Unmarshal(body, &slackResp); err != nil {
		return err
	}

	// ToDo: Check if it's actually a retryable error and only retry those. Currently we retry all errors.
	if !slackResp.Ok {
		return fmt.Errorf(slackResp.Error)
	}

	return nil
}

func NewApp(queueSize int, httpClient *http.Client) *App {

	logger, err := zap.NewProduction()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}

	return &App{
		slackQueue: make(chan SlackPostMessageRequest, queueSize),
		messenger:  &SlackClient{client: httpClient},
		logger:     logger,
	}
}

func (app *App) Shutdown() {
	close(app.slackQueue)
	// Very important to wait, so that we process all the messages in the queue before exiting!
	app.wg.Wait()
}

func (app *App) processQueue(ctx context.Context, MaxRetries int, InitialBackoffMs int, SlackPostMessageURL string, tokenFlag string, burst int) {
	// This is the rate limiter, which will block until it is allowed to continue on r.Wait(ctx).
	// I kept the rate at 1 per second, as doing more than that will cause Slack to reject the messages anyways. We can burst however.
	// Do note that this is best effort, in case of failures, we will exponentially backoff and retry, which will cause the rate to be lower than 1 per second due to obvious reasons.
	r := rate.NewLimiter(rate.Every(1*time.Second), burst)

	for {
		r.Wait(ctx)

		select {
		case msg, ok := <-app.slackQueue:
			// We do check if the channel is closed, but its important to note is that the channel will be closed when the queue is empty and the Shutdown() is called.
			// Simply calling close(app.slackQueue) will not close the channel, it will only prevent any more messages from being sent to the channel.
			// Only once the channel is empty, will it be closed.
			if !ok {
				return
			}
			app.logger.Debug("Got message from queue")

			// Removed this in favour of moving it at the end of the 'loop' myself. This is because after testing, I noticed a massive delay and randomness of when the .Done() was called
			// defer app.wg.Done()
			retryCount := 0
			for {
				err := app.messenger.PostMessage(msg, SlackPostMessageURL, tokenFlag)
				if err != nil {
					app.logger.Error("Failed to post message", zap.Error(err))
					if retryCount < MaxRetries {
						retryCount++
						backoffDuration := time.Duration(InitialBackoffMs*int(math.Pow(2, float64(retryCount-1)))) * time.Millisecond
						time.Sleep(backoffDuration)
					} else {
						app.logger.Error("Message failed after retries", zap.Error(err), zap.Int("retryCount", retryCount))
						break
					}
				} else {
					app.logger.Debug("Message sent successfully")
					break
				}
			}

			// Need to call this to clean up the wg, which is vital for the shutdown to work (so that we process all the messages in the queue before exiting cleanly)
			app.wg.Done()

		case <-ctx.Done():
			return
		}
	}
}
