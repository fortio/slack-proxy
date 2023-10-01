// main.go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

type Metrics struct {
	RequestsRecievedTotal  *prometheus.CounterVec
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
	logger     *zap.Logger
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
	flag.Parse()

	// Initialize metrics
	r := prometheus.NewRegistry()
	metrics := NewMetrics(r)

	// Initialize the app, metrics are passed along so they are accessible
	app := NewApp(maxQueueSize, &http.Client{}, metrics)
	// The only required flag is the token at the moment.
	if tokenFlag == "" {
		app.logger.Fatal("Missing token flag")
	}

	app.logger.Info("Starting up...")
	app.logger.Info("Starting metrics server.")
	go StartMetricServer(r, &MetricsPort)

	// Main ctx
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Server ctx, needed to cancel the server and not every other context
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	app.logger.Info("Starting main app logic")
	go app.processQueue(ctx, MaxRetries, InitialBackoffMs, SlackPostMessageURL, tokenFlag, burst)
	app.logger.Info("Starting reciever server")
	go app.StartServer(serverCtx, &ApplicationPort)

	app.logger.Info("Up and running!")

	// Wait for a shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	app.logger.Info("Shutdown signal received. Cleaning up...")
	app.logger.Info("Shutting down server...")
	serverCancel()
	app.logger.Info("Shutting down queue...")
	app.Shutdown()
	app.logger.Info("Shutdown complete.")
}
