/*
****** WHAT THIS FILE DOES ******
* Entry point of the API Gateway service — the front door of the IICPC platform.
* Every request from contestants passes through here first.
*
* RESPONSIBILITIES:
* - Serves the static submission portal (HTML/CSS/JS) at http://localhost:8080
* - Exposes REST API endpoints for auth, submission and status checks
* - Connects to Redis for submission queue and user sessions
* - Exposes /metrics endpoint for Prometheus scraping
*
* ROUTES:
* - GET  /                     => login/register portal
* - GET  /static/...           => CSS and JS files
* - GET  /health               => health check
* - GET  /metrics              => prometheus scraping
* - POST /auth/register        => register new account
* - POST /auth/login           => login
* - POST /auth/logout          => logout
* - GET  /auth/me              => get current user info
* - POST /submit               => accept code submission
* - GET  /status/:id           => check submission status
* - GET  /submissions          => get submissions for logged in user
* - GET  /admin/stats          => platform stats for admin
* - GET  /admin/users          => all registered users
* - DELETE /admin/users/:username => delete a user
* - GET  /admin/submissions    => all submissions
* - POST /admin/trigger/:id    => re-test a submission
*
* METRICS EXPOSED:
* - api_gateway_http_requests_total            => requests by method, route, status
* - api_gateway_http_request_duration_seconds  => latency by method, route
* - api_gateway_submissions_total              => total submissions received
*
* WHY GIN?
* 40x faster than standard net/http due to radix tree based routing.
*
* WHY PROMETHEUS?
* Scrapes /metrics every 15s — Grafana visualizes platform health in real time.
*/

package main

import (
	"api-gateway/handlers"
	"context"
	"fmt"
	"api-gateway/kafka"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

// ─── Prometheus Metrics ───────────────────────────────────────

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_gateway_http_requests_total",
			Help: "Total HTTP requests by method, route and status",
		},
		[]string{"method", "route", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "api_gateway_http_request_duration_seconds",
			Help:    "Request latency in seconds by method and route",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)

	// incremented in submit handler each time a submission is received
	submissionsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "api_gateway_submissions_total",
			Help: "Total code submissions received",
		},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(submissionsTotal)
}

// ─── Prometheus Middleware ─────────────────────────────────────
// runs on every request — measures duration and tracks status code
func prometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		timer := prometheus.NewTimer(httpRequestDuration.WithLabelValues(
			c.Request.Method,
			c.FullPath(),
		))
		c.Next()
		timer.ObserveDuration()
		httpRequestsTotal.WithLabelValues(
			c.Request.Method,
			c.FullPath(),
			fmt.Sprintf("%d", c.Writer.Status()),
		).Inc()
	}
}

func main() {
	fmt.Println("IICPC Platform - API Gateway starting...")

	// connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: "redis:6379",
	})

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Printf("Redis connection error: %v\n", err)
		return
	}
	fmt.Println("Connected to Redis successfully!")

	handlers.InitRedis()
	handlers.InitAuth(rdb)

	// initialize kafka producer
	kafka.InitProducer()
	defer kafka.CloseProducer()
	
	r := gin.Default()
	r.Use(prometheusMiddleware())

	// observability
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// static files
	r.Static("/static", "./static")
	r.StaticFile("/", "./static/index.html")
	r.StaticFile("/my-submissions", "./static/submissions.html")
	r.StaticFile("/admin", "./static/admin.html")

	// health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "api-gateway", "version": "1.0.0"})
	})

	// auth
	r.POST("/auth/register", handlers.Register)
	r.POST("/auth/login", handlers.Login)
	r.POST("/auth/logout", handlers.Logout)
	r.GET("/auth/me", handlers.GetMe)

	// submissions
	r.POST("/submit", handlers.SubmitCode)
	r.GET("/status/:id", handlers.GetStatus)
	r.GET("/submissions", handlers.GetSubmissions)

	// admin
	r.GET("/admin/stats", handlers.GetStats)
	r.GET("/admin/users", handlers.GetAllUsers)
	r.DELETE("/admin/users/:username", handlers.DeleteUser)
	r.GET("/admin/submissions", handlers.GetAllSubmissions)
	r.POST("/admin/trigger/:id", handlers.TriggerTest)

	fmt.Println("API Gateway running on http://localhost:8080")
	r.Run(":8080")
}