// main.go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"sync"

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
	IconUrl     string          `json:"icon_url,omitempty"`
	IconEmoji   string          `json:"icon_emoji,omitempty"`
	ThreadTs    string          `json:"thread_ts,omitempty"`
	Parse       string          `json:"parse,omitempty"`
	LinkNames   bool            `json:"link_names,omitempty"`
	Blocks      json.RawMessage `json:"blocks,omitempty"`      // JSON serialized array of blocks
	Attachments json.RawMessage `json:"attachments,omitempty"` // JSON serialized array of attachments
}

type App struct {
	slackQueue chan SlackPostMessageRequest
	wg         sync.WaitGroup
	messenger  SlackMessenger
	metrics    *Metrics
}

func main() {
	var (
		MaxRetries          = 2
		InitialBackoffMs    = 1000
		SlackPostMessageURL = "https://slack.com/api/chat.postMessage"
		maxQueueSize        = 100
		burst               = 3
		tokenFlag           string
		MetricsPort         = ":9090"
		ApplicationPort     = ":8080"
	)

	// Define the flags with the default values
	flag.IntVar(&MaxRetries, "maxRetries", MaxRetries, "Maximum number of retries for posting a message")
	flag.IntVar(&InitialBackoffMs, "initialBackoffMs", InitialBackoffMs, "Initial backoff in milliseconds for retries")
	flag.StringVar(&SlackPostMessageURL, "slackURL", SlackPostMessageURL, "Slack Post Message API URL")
	flag.IntVar(&maxQueueSize, "queueSize", maxQueueSize, "Maximum number of messages in the queue")
	flag.IntVar(&burst, "burst", burst, "Maximum number of burst to allow")
	flag.StringVar(&tokenFlag, "token", "", "Bearer token for the Slack API")
	flag.StringVar(&MetricsPort, "metricsPort", MetricsPort, "Port for the metrics server")
	flag.StringVar(&ApplicationPort, "applicationPort", ApplicationPort, "Port for the application server")

	scli.ServerMain()

	// Initialize metrics
	r := prometheus.NewRegistry()
	metrics := NewMetrics(r)

	// Initialize the app, metrics are passed along so they are accessible
	app := NewApp(maxQueueSize, &http.Client{}, metrics)
	// The only required flag is the token at the moment.
	if tokenFlag == "" {
		log.Fatalf("Missing token flag")
	}

	log.Infof("Starting metrics server.")
	go StartMetricServer(r, &MetricsPort)

	// Main ctx
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Server ctx, needed to cancel the server and not every other context
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	log.Infof("Starting main app logic")
	go app.processQueue(ctx, MaxRetries, InitialBackoffMs, SlackPostMessageURL, tokenFlag, burst)
	log.Infof("Starting receiver server")
	go app.StartServer(serverCtx, &ApplicationPort)

	log.Infof("Up and running!")

	// Shutdown is handled by scli
	scli.UntilInterrupted()
	log.Infof("Shutting down server...")
	serverCancel()
	log.Infof("Shutting down queue...")
	app.Shutdown()
	log.Infof("Shutdown complete.")
}
