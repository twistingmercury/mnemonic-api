package server

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "github.com/twistingmercury/mnemonic-api/docs/swagger"
	"github.com/twistingmercury/mnemonic-api/internal/config"
	agenthandler "github.com/twistingmercury/mnemonic-api/internal/handlers/agents"
	patternhandler "github.com/twistingmercury/mnemonic-api/internal/handlers/patterns"
	skillfilehandler "github.com/twistingmercury/mnemonic-api/internal/handlers/skillfiles"
	skillhandler "github.com/twistingmercury/mnemonic-api/internal/handlers/skills"
	agentsvc "github.com/twistingmercury/mnemonic-api/internal/service/agent"
	patternsvc "github.com/twistingmercury/mnemonic-api/internal/service/pattern"
	searchsvc "github.com/twistingmercury/mnemonic-api/internal/service/search"
	skillsvc "github.com/twistingmercury/mnemonic-api/internal/service/skill"
	skillfilesvc "github.com/twistingmercury/mnemonic-api/internal/service/skillfile"
)

// Services groups all domain services required by the REST API handlers.
type Services struct {
	Agent     agentsvc.Service
	Pattern   patternsvc.Service
	Search    searchsvc.Service
	Skill     skillsvc.Service
	SkillFile skillfilesvc.Service
}

// RegisterAPIRoutes creates all domain handlers and registers their routes
// on the /v1/api route group. Call this after setting up middleware.
func RegisterAPIRoutes(router *gin.Engine, svc Services, vocab config.VocabularyConfig) {
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	v1 := router.Group("/v1/api")

	agenthandler.New(svc.Agent).RegisterRoutes(v1)
	patternhandler.New(svc.Pattern, svc.Search, vocab).RegisterRoutes(v1)
	skillhandler.New(svc.Skill).RegisterRoutes(v1)
	skillfilehandler.New(svc.SkillFile).RegisterRoutes(v1)
}
