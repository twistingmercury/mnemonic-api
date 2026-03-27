package server

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "github.com/twistingmercury/mnemonic-api/docs/swagger"
	"github.com/twistingmercury/mnemonic-api/internal/config"
	patternhandler "github.com/twistingmercury/mnemonic-api/internal/handlers/patterns"
	patternsvc "github.com/twistingmercury/mnemonic-api/internal/service/pattern"
	searchsvc "github.com/twistingmercury/mnemonic-api/internal/service/search"
)

// Services groups all domain services required by the REST API handlers.
type Services struct {
	Pattern patternsvc.Service
	Search  searchsvc.Service
}

// RegisterAPIRoutes creates all domain handlers and registers their routes
// on the /v1/api route group. Call this after setting up middleware.
func RegisterAPIRoutes(router *gin.Engine, svc Services, vocab config.VocabularyConfig) {
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	v1 := router.Group("/v1/api")

	patternhandler.New(svc.Pattern, svc.Search, vocab).RegisterRoutes(v1)
}
