package main

import (
	"fortio.org/fortio/fhttp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		RequestsReceivedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "slackproxy",
				Name:      "requests_received_total",
				Help:      "The total number of requests received",
			},
			[]string{"channel"},
		),
		RequestsFailedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "slackproxy",
				Name:      "requests_failed_total",
				Help:      "The total number of requests failed",
			},
			[]string{"channel"},
		),
		RequestsRetriedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "slackproxy",
				Name:      "requests_retried_total",
				Help:      "The total number of requests retried",
			},
			[]string{"channel"},
		),
		RequestsSucceededTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "slackproxy",
				Name:      "requests_succeeded_total",
				Help:      "The total number of requests retried",
			},
			[]string{"channel"},
		),
		RequestsNotProcessed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "slackproxy",
				Name:      "requests_not_processed_total",
				Help:      "The total number of requests not processed",
			},
			[]string{"channel"},
		),
		QueueSize: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "slackproxy",
				Name:      "queue_size",
				Help:      "The current size of the queue",
			},
			nil,
		),
	}

	reg.MustRegister(m.RequestsReceivedTotal)
	reg.MustRegister(m.RequestsFailedTotal)
	reg.MustRegister(m.RequestsRetriedTotal)
	reg.MustRegister(m.RequestsSucceededTotal)
	reg.MustRegister(m.RequestsNotProcessed)
	reg.MustRegister(m.QueueSize)

	return m
}

func StartMetricServer(reg *prometheus.Registry, addr string) {
	mux, _ := fhttp.HTTPServer("metrics", addr)
	mux.Handle("/metrics", promhttp.HandlerFor(
		reg,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
			// Pass custom registry
			Registry: reg,
		},
	))
}
