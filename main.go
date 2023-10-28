// main.go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"sync"
	"time"

	"fortio.org/log"
	"fortio.org/scli"
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	RequestsReceivedTotal  *prometheus.CounterVec
	RequestsFailedTotal    *prometheus.CounterVec
	RequestsRetriedTotal   *prometheus.CounterVec
	RequestsSucceededTotal *prometheus.CounterVec
	RequestsNotProcessed   *prometheus.CounterVec
	QueueSize              *prometheus.GaugeVec
}

type SlackResponse struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type SlackPostMessageRequest struct {
	Token       string          `json:"token"`
	Channel     string          `json:"channel"`
	Text        string          `json:"text"`
	AsUser      bool            `json:"as_user,omitempty"`
	Username    string          `json:"username,omitempty"`
	IconURL     string          `json:"icon_url,omitempty"`
	IconEmoji   string          `json:"icon_emoji,omitempty"`
	ThreadTS    string          `json:"thread_ts,omitempty"`
	Parse       string          `json:"parse,omitempty"`
	LinkNames   bool            `json:"link_names,omitempty"`
	Blocks      json.RawMessage `json:"blocks,omitempty"`      // JSON serialized array of blocks
	Attachments json.RawMessage `json:"attachments,omitempty"` // JSON serialized array of attachments
}

type App struct {
	slackQueue      chan SlackPostMessageRequest
	wg              sync.WaitGroup
	messenger       SlackMessenger
	metrics         *Metrics
	channelOverride string
}

func main() {
	var (
		maxRetries          = 2
		initialBackoffMs    = 1000
		slackPostMessageURL = "https://slack.com/api/chat.postMessage"
		maxQueueSize        = 100
		burst               = 3
		token               string
		metricsPort         = ":9090"
		applicationPort     = ":8080"
		channelOverride     string
	)

	// Define the flags with the default values // TODO: move the ones that can change to dflag
	flag.IntVar(&maxRetries, "maxRetries", maxRetries, "Maximum number of retries for posting a message")
	flag.IntVar(&initialBackoffMs, "initialBackoffMs", initialBackoffMs, "Initial backoff in milliseconds for retries")
	flag.StringVar(&slackPostMessageURL, "slackURL", slackPostMessageURL, "Slack Post Message API URL")
	flag.IntVar(&maxQueueSize, "queueSize", maxQueueSize, "Maximum number of messages in the queue")
	flag.IntVar(&burst, "burst", burst, "Maximum number of burst to allow")
	flag.StringVar(&metricsPort, "metricsPort", metricsPort, "Port for the metrics server")
	flag.StringVar(&applicationPort, "applicationPort", applicationPort, "Port for the application server")
	flag.StringVar(&channelOverride, "channelOverride", "", "Override the channel for all messages - Be careful with this one!")

	scli.ServerMain()

	token = os.Getenv("SLACK_TOKEN")
	if token == "" {
		log.Fatalf("SLACK_TOKEN environment variable not set")
	}

	// Initialize metrics
	r := prometheus.NewRegistry()
	metrics := NewMetrics(r)

	// Initialize the app, metrics are passed along so they are accessible
	app := NewApp(maxQueueSize, &http.Client{
		Timeout: 10 * time.Second,
	}, metrics, channelOverride)

	log.Infof("Starting metrics server.")
	StartMetricServer(r, metricsPort)

	// Main ctx
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Server ctx, needed to cancel the server and not every other context
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	log.Infof("Starting main app logic")
	go app.processQueue(ctx, maxRetries, initialBackoffMs, slackPostMessageURL, token, burst)
	log.Infof("Starting receiver server")
	// Check error return of app.StartServer in go routine anon function:
	go func() {
		err := app.StartServer(serverCtx, applicationPort)
		if err != nil {
			log.Fatalf("Error starting server: %v", err)
		}
	}()

	log.Infof("Up and running!")

	// Shutdown is handled by scli
	scli.UntilInterrupted()
	log.Infof("Shutting down server...")
	serverCancel()
	log.Infof("Shutting down queue...")
	app.Shutdown()
	log.Infof("Shutdown complete.")
}
