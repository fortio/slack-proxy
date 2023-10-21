// app_test.go

package main

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"fortio.org/log"
	"github.com/prometheus/client_golang/prometheus"
)

type MockSlackMessenger struct {
	shouldError bool
}

func (m *MockSlackMessenger) PostMessage(req SlackPostMessageRequest, url string, token string) error {
	if m.shouldError {
		return errors.New("mock error")
	}
	return nil
}

func TestApp_singleBurst_Success(t *testing.T) {
	r := prometheus.NewRegistry()
	metrics := NewMetrics(r)

	messenger := &MockSlackMessenger{}
	app := &App{
		slackQueue: make(chan SlackPostMessageRequest, 2),
		messenger:  messenger,
		metrics:    metrics,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go app.processQueue(ctx, 3, 1000, "http://mock.url", "mockToken", 1)

	startTime := time.Now()

	count := 10
	for i := 0; i < count; i++ {
		app.wg.Add(1)
		app.slackQueue <- SlackPostMessageRequest{
			Channel: "mockChannel",
		}
	}

	log.S(log.Debug, "Posting messages done")

	app.wg.Wait()

	endTime := time.Now()

	diffInSeconds := endTime.Sub(startTime).Seconds()
	log.S(log.Debug, "diffInSeconds", log.Float64("diffInSeconds", diffInSeconds))

	// The sum is always: (Amount of messages * delay in seconds) minus burst. In this case 10 * 1 - 1 = 9 seconds.
	if math.RoundToEven(diffInSeconds) != 9 {
		t.Fatal("Expected processQueue finish the job in ~9 seconds, give or take. Got", diffInSeconds)
	}
}

func TestApp_MultiBurst_Success(t *testing.T) {
	r := prometheus.NewRegistry()
	metrics := NewMetrics(r)

	messenger := &MockSlackMessenger{}
	app := &App{
		slackQueue: make(chan SlackPostMessageRequest, 2),
		messenger:  messenger,
		metrics:    metrics,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go app.processQueue(ctx, 3, 1000, "http://mock.url", "mockToken", 10)

	startTime := time.Now()

	count := 20
	for i := 0; i < count; i++ {
		app.wg.Add(1)
		app.slackQueue <- SlackPostMessageRequest{
			Channel: "mockChannel",
		}
	}

	log.S(log.Debug, "Posting messages done")

	app.wg.Wait()

	endTime := time.Now()

	diffInSeconds := endTime.Sub(startTime).Seconds()
	log.S(log.Debug, "diffInSeconds", log.Float64("diffInSeconds", diffInSeconds))

	// The sum is always: (Amount of messages * delay in seconds) minus burst. In this case 20 * 1 - 10 = 10 seconds.
	if math.RoundToEven(diffInSeconds) != 10 {
		t.Fatal("Expected processQueue finish the job in ~9 seconds, give or take. Got", diffInSeconds)
	}
}
