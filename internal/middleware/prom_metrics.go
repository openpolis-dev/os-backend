package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	requestCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "seedao",
		Subsystem: "osbackend",
		Name:      "request_count",
		Help:      "Total number of requests",
	})

	requestDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "seedao",
		Subsystem: "osbackend",
		Name:      "request_duration",
		Help:      "Duration of requests",
	})

	requestSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "seedao",
		Subsystem: "osbackend",
		Name:      "request_size",
		Help:      "Size of requests",
	})

	responseSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "seedao",
		Subsystem: "osbackend",
		Name:      "response_size",
		Help:      "Size of responses",
	})
)

func IsSeedaoRequest(fullPath string) bool {
	return strings.HasPrefix(fullPath, "/v")
}

func RequestMetricsRecord() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if IsSeedaoRequest(ctx.FullPath()) {
			requestCount.Inc()
			requestSize.Observe(float64(ctx.Request.ContentLength))

			ctx.Next()
			// Calculate request and response time
			startTime := time.Now()
			ctx.Next()
			elapsed := time.Since(startTime).Milliseconds()

			requestDuration.Observe(float64(elapsed))
		} else {
			ctx.Next()
		}
	}
}

func ResponseMetricsRecord() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if IsSeedaoRequest(ctx.FullPath()) {
			responseSize.Observe(float64(ctx.Writer.Size()))
		}
	}
}
