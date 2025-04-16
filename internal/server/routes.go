package server

import (
	"harvest/internal/controller"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func (s *Server) RegisterRoutes() http.Handler {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     s.config.CORS.AllowedOrigins,
		AllowMethods:     s.config.CORS.AllowedMethods,
		AllowHeaders:     s.config.CORS.AllowedHeaders,
		AllowCredentials: s.config.CORS.AllowCredentials,
		MaxAge:           time.Duration(s.config.CORS.MaxAge) * time.Second,
	}))

	// Public routes that don't require authentication
	r.GET("/ready", s.readyHandler)
	r.GET("/online", s.onlineHandler)

	// API routes that require either ADMIN or SERVICE role
	api := r.Group("/api")
	api.Use(s.AuthMiddleware(controller.RoleAdmin, controller.RoleService))
	{
		api.POST("/names", s.namesHandler)
		api.POST("/buildMatches", s.BuildMatchesFromFilter)
		api.POST("/tournaments", s.tournamentsHandler)

		// Job routes - accessible to both ADMIN and SERVICE roles
		jobs := api.Group("/jobs")
		{
			jobs.POST("", s.CreateJobHandler)
			jobs.GET("", s.ListJobsHandler)
			jobs.GET("/all", s.ListAllJobsHandler) // For listing all jobs with optional status filter
			jobs.GET("/types", s.ListAllAvailableJobTypes)
			jobs.GET("/:id", s.GetJobHandler)
		}

		metrics := api.Group("/metrics")
		{
			metrics.GET("/total", s.GetTotalMatchCountHandler)
			metrics.GET("/distribution", s.GetMatchDistributionHandler)
			metrics.GET("/time-range", s.GetMatchMetricsForTimeRangeHandler)
		}
	}

	// Token management routes (ADMIN only)
	tokens := r.Group("/token")
	tokens.Use(s.AuthMiddleware(controller.RoleAdmin))
	{
		tokens.POST("", s.CreateTokenHandler)
		tokens.GET("", s.ListTokensHandler)
		tokens.GET("/:id", s.GetTokenHandler)
		tokens.DELETE("/:id", s.RevokeTokenHandler)
	}

	return r
}
