package api

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/aibao/server/internal/api/middleware"
	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/pkg/jwtauth"
)

// RouterDeps groups everything NewRouter needs from main.go. Pulling these
// into a struct keeps the call site stable as we add more dependencies.
type RouterDeps struct {
	Metrics *metrics.Metrics
	Reg     *prometheus.Registry
	PG      Checker
	Redis   Checker

	// Auth-related (Plan 2)
	JWT   *jwtauth.Manager
	Auth  *AuthHandler
	Me    *MeHandler
	Child *ChildHandler

	// Story generation (Plan 4)
	Story        *StoryHandler
	GenRateLimit gin.HandlerFunc
	BudgetGuard  gin.HandlerFunc
}

// NewRouter builds the gin.Engine with the standard middleware chain,
// health/ready endpoints, /metrics scrape, and (when Plan 2 deps are
// supplied) the /api/v1 routes split into public and JWT-protected groups.
// Order of middleware matters: Recover must be outermost so it can catch
// panics from any later middleware or handler.
func NewRouter(deps RouterDeps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(
		middleware.Recover(),
		middleware.TraceID(),
		middleware.Logger(),
		middleware.Metrics(deps.Metrics),
	)

	RegisterHealth(r, deps.PG, deps.Redis)

	r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(deps.Reg, promhttp.HandlerOpts{})))

	// Public v1 routes
	v1 := r.Group("/api/v1")
	if deps.Auth != nil {
		deps.Auth.RegisterRoutes(v1)
	}

	// Authenticated v1 routes
	if deps.JWT != nil {
		auth := r.Group("/api/v1")
		auth.Use(middleware.JWTAuth(deps.JWT))
		if deps.Me != nil {
			deps.Me.RegisterRoutes(auth)
		}
		if deps.Child != nil {
			deps.Child.RegisterRoutes(auth)
		}
		if deps.Story != nil {
			gen := auth.Group("")
			if deps.GenRateLimit != nil {
				gen.Use(deps.GenRateLimit)
			}
			if deps.BudgetGuard != nil {
				gen.Use(deps.BudgetGuard)
			}
			deps.Story.RegisterRoutes(gen)
		}
	}

	return r
}
