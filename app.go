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

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
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

	if !slackResp.Ok {
		return fmt.Errorf(slackResp.Error)
	}

	return nil
	// return fmt.Errorf("error")

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
	app.wg.Wait()
}

func (app *App) processQueue(ctx context.Context, MaxRetries int, InitialBackoffMs int, SlackPostMessageURL string, tokenFlag string, burst int) {
	r := rate.NewLimiter(rate.Every(1*time.Second), burst)

	for {
		r.Wait(ctx)

		select {
		case msg, ok := <-app.slackQueue:

			if app.logger == nil {
				fmt.Println("logger is nil")
			}
			app.logger.Debug("Got message from queue")
			if !ok {
				return
			}

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
						app.logger.Error("Message failed after retries", zap.Error(err))
						break
					}
				} else {
					app.logger.Info("Message sent successfully")
					break
				}
			}

			app.wg.Done()

			app.logger.Debug("I should move out of the function now")

		case <-ctx.Done():
			return
		}
	}
}
