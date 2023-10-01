package main

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewMetrics(reg prometheus.Registerer) *Metrics {

	m := &Metrics{
		RequestsRecievedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "requests_recieved_total",
				Help: "The total number of requests recieved",
			},
			[]string{"channel"},
		),
		RequestsFailedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "requests_failed_total",
				Help: "The total number of requests failed",
			},
			[]string{"channel"},
		),
		RequestsRetriedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "requests_retried_total",
				Help: "The total number of requests retried",
			},
			[]string{"channel"},
		),
		RequestsSucceededTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "requests_succeeded_total",
				Help: "The total number of requests retried",
			},
			[]string{"channel"},
		),
		QueueSize: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "queue_size",
				Help: "The current size of the queue",
			},
			nil,
		),
	}

	reg.MustRegister(m.RequestsRecievedTotal)
	reg.MustRegister(m.RequestsFailedTotal)
	reg.MustRegister(m.RequestsRetriedTotal)
	reg.MustRegister(m.RequestsSucceededTotal)
	reg.MustRegister(m.QueueSize)

	return m

}

func StartMetricServer(reg *prometheus.Registry, addr *string) {

	http.Handle("/metrics", promhttp.HandlerFor(
		reg,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
			// Pass custom registry
			Registry: reg,
		},
	))
	log.Fatal(http.ListenAndServe(*addr, nil))
}
