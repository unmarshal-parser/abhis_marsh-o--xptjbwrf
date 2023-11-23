// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"os"
	"time"
)

var indexerConfig IndexerConfig

func init() {
	configFilePath := os.Getenv("CONFIG_FILE_PATH")
	if len(configFilePath) == 0 {
		panic("missing CONFIG_FILE_PATH value")
	}
	err := LoadConfig("run", configFilePath, &indexerConfig)
	if err != nil {
		panic(err)
	}
	indexerConfig.AssignDefaults()
	indexerConfig.Metrics = initAndAssignMetrics()
}

func initAndAssignMetrics() *Metrics {
	return &Metrics{
		NodeRestarts: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: indexerConfig.PrometheusNameSpace,
			Subsystem: indexerConfig.PrometheusSubsystem,
			Name:      "node_restart",
			Help:      "Count simulated restarts due to node",
		}, []string{
			"location",
			"node_endpoint",
			"error",
		}),
		NodeLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: indexerConfig.PrometheusNameSpace,
			Subsystem: indexerConfig.PrometheusSubsystem,
			Name:      "node_latency",
			Help:      "Response latency from the node",
		}, []string{
			"node_endpoint",
			"method",
		}),
		UnmarshalAPIRestarts: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: indexerConfig.PrometheusNameSpace,
			Subsystem: indexerConfig.PrometheusSubsystem,
			Name:      "api_gateway_restart",
			Help:      "Count simulated restarts due to API Gateway",
		}, []string{
			"location",
			"chain",
		}),
		UnmarshalAPILatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: indexerConfig.PrometheusNameSpace,
			Subsystem: indexerConfig.PrometheusSubsystem,
			Name:      "api_latency",
			Help:      "Response latency from API Gateway",
		}, []string{
			"chain",
			"api",
		}),

		TokenStoreFailure: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: indexerConfig.PrometheusNameSpace,
			Subsystem: indexerConfig.PrometheusSubsystem,
			Name:      "tokenstore_failure",
			Help:      "Count simulated restarts due to API Gateway",
		}, []string{
			"api",
			"contract_address",
			"chain",
		}),
		TokenStoreLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: indexerConfig.PrometheusNameSpace,
			Subsystem: indexerConfig.PrometheusSubsystem,
			Name:      "tokenstore_latency",
			Help:      "Response latency from API Gateway",
		}, []string{
			"api",
			"contract_address",
			"chain",
		}),
		PriceStoreFailure: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: indexerConfig.PrometheusNameSpace,
			Subsystem: indexerConfig.PrometheusSubsystem,
			Name:      "pricestore_failure",
			Help:      "Count simulated restarts due to API Gateway",
		}, []string{
			"api",
			"data_passed",
			"timestamp_price_fetched_for",
		}),
		PriceStoreLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: indexerConfig.PrometheusNameSpace,
			Subsystem: indexerConfig.PrometheusSubsystem,
			Name:      "pricestore_latency",
			Help:      "Response latency from API Gateway",
		}, []string{
			"api",
			"data_passed",
			"timestamp_price_fetched_for",
		}),
	}
}

func main() {
	go RunPrometheusServer()
	for true {
		log.Info("Starting Service")
		RunIndexer(indexerConfig)
		log.Info("Exited Service. Simulating restart. Sleeping before resumption...")
		time.Sleep(time.Second * 15)
	}
}

func PromHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func RunPrometheusServer() {
	var metricsRouter = gin.Default()
	metricsRouter.GET("/metrics", PromHandler())
	err := metricsRouter.Run(fmt.Sprintf("%s:%d", indexerConfig.PrometheusHost, indexerConfig.MetricsPort))
	if err != nil {
		log.WithField("Error", err).Error("metrics server shut down")
		panic(err)
	}
}
