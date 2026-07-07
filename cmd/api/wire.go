//go:build wireinject
// +build wireinject

package main

import (
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/gemini"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/handler"
	"github.com/google/wire"
	"github.com/labstack/echo/v4"
)

// InitializeAPI wires up the entire application and returns the Echo router.
// NOTE FOR ANTIGRAVITY AGENT:
// You must expand this provider set to include Database, Redis, Asynq, and Repositories
// before running `make wire`.
func InitializeAPI(geminiAPIKey string, geminiModel string, maxConcurrent int64) (*echo.Echo, error) {
	wire.Build(
		// 1. Infrastructure Providers
		gemini.NewClient,
		// Bind the concrete *gemini.Client to the AIGeneratorService interface
		wire.Bind(new(assessment.AIGeneratorService), new(*gemini.Client)),

		// [ANTIGRAVITY TODO: Add DB, Redis, Asynq Client providers here]
		// [ANTIGRAVITY TODO: Add Repository implementations here]

		// 2. Application (Usecase) Providers
		assessment.NewSubmitAssessmentUseCase,
		// [ANTIGRAVITY TODO: Add other usecases here]

		// 3. Delivery (HTTP) Providers
		handler.NewAssessmentHandler,
		http.SetupRouter,
	)
	return nil, nil
}
