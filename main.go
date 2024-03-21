// main.go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	slackQueue          chan SlackPostMessageRequest
	wg                  sync.WaitGroup
	messenger           SlackMessenger
	SlackPostMessageURL string
	SlackToken          string
	metrics             *Metrics
	channelOverride     string
}

// podIndex retrieves the index of the current pod based on the HOSTNAME environment variable.
// The function expects the HOSTNAME to be in the format <name>-<index>.
// It returns the index as an integer and an error if any occurred during the process.
// If the HOSTNAME environment variable is not set or if the format is invalid, it returns an error.
func podIndex(podName string) (int, error) {
	lastDash := strings.LastIndex(podName, "-")
	if lastDash == -1 || lastDash == len(podName)-1 {
		return 0, fmt.Errorf("invalid pod name %s. Expected <name>-<index>", podName)
	}

	indexStr := podName[lastDash+1:]
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return 0, fmt.Errorf("invalid pod name format. Expected <name>-<index>, got %s", podName)
	}

	return index, nil
}

func getSlackTokens() []string {
	tokensEnv := os.Getenv("SLACK_TOKENS")
	if tokensEnv == "" {
		return []string{}
	}

	tokens := strings.Split(tokensEnv, ",")
	for i, token := range tokens {
		tokens[i] = strings.TrimSpace(token)
	}

	return tokens
}

func main() {
	var (
		maxRetries          = 2
		slackPostMessageURL = "https://slack.com/api/chat.postMessage"
		maxQueueSize        = 100
		burst               = 3
		metricsPort         = ":9090"
		applicationPort     = ":8080"
		channelOverride     string
	)

	initialBackoff := flag.Duration("initialBackoff", 1000*time.Millisecond, "Initial backoff in milliseconds for retries")
	slackRequestRate := flag.Duration("slackRequestRate", 1000*time.Millisecond, "Rate limit for slack requests in milliseconds")

	// Define the flags with the default values // TODO: move the ones that can change to dflag
	flag.IntVar(&maxRetries, "maxRetries", maxRetries, "Maximum number of retries for posting a message")
	flag.StringVar(&slackPostMessageURL, "slackURL", slackPostMessageURL, "Slack Post Message API URL")
	flag.IntVar(&maxQueueSize, "queueSize", maxQueueSize, "Maximum number of messages in the queue")
	flag.IntVar(&burst, "burst", burst, "Maximum number of burst to allow")
	flag.StringVar(&metricsPort, "metricsPort", metricsPort, "Port for the metrics server")
	flag.StringVar(&applicationPort, "applicationPort", applicationPort, "Port for the application server")
	flag.StringVar(&channelOverride, "channelOverride", "", "Override the channel for all messages - Be careful with this one!")

	scli.ServerMain()

	// Get list of comma separated tokens from environment variable SLACK_TOKENS
	tokens := getSlackTokens()

	// Hack to get the pod index
	// Todo: Remove this by using the label pod-index:
	// https://github.com/kubernetes/kubernetes/pull/119232
	podName := os.Getenv("HOSTNAME")
	if podName == "" {
		log.Fatalf("HOSTNAME environment variable not set")
	}

	index, err := podIndex(podName)
	if err != nil {
		log.Fatalf("Failed to get pod index: %v", err)
	}

	// Get the token for the current pod
	// If the index is out of range, we fail
	log.S(log.Info, "Pod", log.Any("index", index), log.Any("num-tokens", len(tokens)))
	if index >= len(tokens) {
		log.Fatalf("Pod index %d is out of range for the list of %d tokens", index, len(tokens))
	}
	token := tokens[index]

	// Initialize metrics
	r := prometheus.NewRegistry()
	metrics := NewMetrics(r)

	// Initialize the app, metrics are passed along so they are accessible
	app := NewApp(maxQueueSize, &http.Client{
		Timeout: 10 * time.Second,
	}, metrics, channelOverride, slackPostMessageURL, token)

	log.Infof("Starting metrics server.")
	StartMetricServer(r, metricsPort)

	// Main ctx
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Server ctx, needed to cancel the server and not every other context
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	log.Infof("Starting main app logic")
	go app.processQueue(ctx, maxRetries, *initialBackoff, burst, *slackRequestRate)
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
