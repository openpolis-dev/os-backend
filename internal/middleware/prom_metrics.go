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

	getRequestCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "seedao",
		Subsystem: "osbackend",
		Name:      "get_request_count",
		Help:      "Total number of get requests",
	})

	postRequestCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "seedao",
		Subsystem: "osbackend",
		Name:      "post_request_count",
		Help:      "Total number of post requests",
	})

	requestDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "seedao",
		Subsystem: "osbackend",
		Name:      "request_duration",
		Help:      "Duration of requests",
		Buckets:   prometheus.LinearBuckets(0, 50, 40),
	})

	postBodySize = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "seedao",
		Subsystem: "osbackend",
		Name:      "post_body_size",
		Help:      "Body size of post request",
		Buckets:   prometheus.LinearBuckets(0, 1000, 10),
	})

	responseSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "seedao",
		Subsystem: "osbackend",
		Name:      "response_size",
		Help:      "Size of responses",
		Buckets:   prometheus.LinearBuckets(0, 1000, 10),
	})

	errorResponseCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "seedao",
		Subsystem: "osbackend",
		Name:      "error_response_count",
		Help:      "Total number of error responses",
	})

	response2xxCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "seedao",
		Subsystem: "osbackend",
		Name:      "response_2xx_count",
		Help:      "Total number of 2xx responses",
	})

	response3xxCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "seedao",
		Subsystem: "osbackend",
		Name:      "response_3xx_count",
		Help:      "Total number of 3xx responses",
	})

	response4xxCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "seedao",
		Subsystem: "osbackend",
		Name:      "response_4xx_count",
		Help:      "Total number of 4xx responses",
	})

	response5xxCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "seedao",
		Subsystem: "osbackend",
		Name:      "response_5xx_count",
		Help:      "Total number of 5xx responses",
	})
)

func IsSeedaoRequest(fullPath string) bool {
	return strings.HasPrefix(fullPath, "/v")
}

func RequestMetricsRecord() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		startTime := time.Now()
		if IsSeedaoRequest(ctx.FullPath()) {
			requestCount.Inc()
			if ctx.Request.Method == "GET" {
				getRequestCount.Inc()
			} else if ctx.Request.Method == "POST" {
				postRequestCount.Inc()
				postBodySize.Observe(float64(ctx.Request.ContentLength))
			}

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
		ctx.Next()
		if IsSeedaoRequest(ctx.FullPath()) {
			responseBodySize := ctx.Writer.Size()
			// -1 means no response body is written
			if responseBodySize != -1 {
				responseSize.Observe(float64(responseBodySize))
			}

			// Get the response status code
			statusCode := ctx.Writer.Status()

			// Increment the corresponding response count based on the status code range
			switch {
			case statusCode >= 200 && statusCode < 300:
				response2xxCount.Inc()
			case statusCode >= 300 && statusCode < 400:
				response3xxCount.Inc()
			case statusCode >= 400 && statusCode < 500:
				response4xxCount.Inc()
				errorResponseCount.Inc()
			case statusCode >= 500 && statusCode < 600:
				response5xxCount.Inc()
				errorResponseCount.Inc()
			}
		}
	}
}
