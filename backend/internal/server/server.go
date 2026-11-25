// Package server wires the production HTTP API and its services.
package server

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"devsec-contest01/internal/config"
	dbsqlc "devsec-contest01/internal/db/sqlc"
	"devsec-contest01/internal/handler"
	"devsec-contest01/internal/middleware"
	"devsec-contest01/internal/service"
	"devsec-contest01/internal/tasks"
)

// New builds the API and initializes the shared clock without resetting existing state.
func New(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) (*gin.Engine, error) {
	queries := dbsqlc.New(pool)
	registry := tasks.NewRegistry()

	authSvc := service.NewAuth(queries, time.Duration(cfg.SessionTTLHours)*time.Hour)
	clockSvc := service.NewClock(queries, cfg.ContestDurationSeconds)
	selectionSvc := service.NewSelection(queries, registry)
	draftsSvc := service.NewDrafts(queries, registry, selectionSvc, pool)
	judgeSvc := service.NewJudge(queries, registry, selectionSvc)
	leaderboardSvc := service.NewLeaderboard(queries)

	if err := clockSvc.InitContestState(ctx); err != nil {
		return nil, err
	}

	authHandler := handler.NewAuth(authSvc)
	tasksHandler := handler.NewTasks(registry, selectionSvc)
	selectionHandler := handler.NewSelection(selectionSvc)
	submissionHandler := handler.NewSubmission(judgeSvc)
	leaderboardHandler := handler.NewLeaderboard(leaderboardSvc, registry)
	clockHandler := handler.NewClock(clockSvc)
	healthHandler := handler.NewHealth(pool)
	draftsHandler := handler.NewDrafts(draftsSvc)
	analyzerSocket := cfg.AnalyzerSocket
	if analyzerSocket == "" {
		analyzerSocket = "/run/contest-analyzers/lsp.sock"
	}
	assistHandler := handler.NewAssist(analyzerSocket, selectionSvc, registry)
	runnerSocket := cfg.RunnerSocket
	if runnerSocket == "" {
		runnerSocket = "/run/contest-runner/run.sock"
	}
	executionHandler := handler.NewExecution(service.NewExecution(pool, registry, selectionSvc, runnerSocket))

	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", healthHandler.Check)

	api := r.Group("/api/v1")
	api.POST("/register", authHandler.Register)
	api.GET("/tasks", tasksHandler.List)
	api.GET("/leaderboard", leaderboardHandler.List)
	api.GET("/clock", clockHandler.Get)

	participantAuth := middleware.ParticipantAuth(authSvc)
	eventLimit := middleware.ParticipantEventLimit()
	api.GET("/me", participantAuth, selectionHandler.Me)
	api.GET("/tasks/:taskId/input", participantAuth, tasksHandler.Input)
	api.POST("/tasks/:taskId/select", participantAuth, selectionHandler.Select)
	api.POST("/tasks/:taskId/submit", participantAuth, submissionHandler.Submit)
	api.GET("/tasks/:taskId/draft", participantAuth, draftsHandler.Get)
	api.PUT("/tasks/:taskId/draft", participantAuth, draftsHandler.Save)
	api.POST("/tasks/:taskId/assist", participantAuth, assistHandler.Handle)
	api.POST("/tasks/:taskId/runs", participantAuth, executionHandler.Create)
	api.GET("/tasks/:taskId/runs/:runId", participantAuth, executionHandler.Get)
	api.POST("/tasks/:taskId/events", participantAuth, eventLimit, draftsHandler.RecordEvent)

	adminAuth := middleware.AdminAuth(cfg.AdminToken)
	api.GET("/admin/participants", adminAuth, draftsHandler.SearchParticipants)
	api.POST("/admin/clock/start", adminAuth, clockHandler.Start)
	api.GET("/admin/participants/:participantId/tasks/:taskId/history", adminAuth, draftsHandler.ListHistory)
	api.GET("/admin/participants/:participantId/tasks/:taskId/history/:version", adminAuth, draftsHandler.GetHistoryVersion)
	api.GET("/admin/participants/:participantId/tasks/:taskId/events", adminAuth, draftsHandler.ListEvents)
	api.GET("/admin/participants/:participantId/tasks/:taskId/timing", adminAuth, draftsHandler.GetTaskTiming)

	if err := mountFrontend(r, cfg.StaticDir); err != nil {
		return nil, err
	}
	return r, nil

}
