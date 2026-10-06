package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterPixLabWorkbenchRoutes registers the self-authenticated workbench
// surface before the normal WeKnora JWT/API-key middleware.
func RegisterPixLabWorkbenchRoutes(r *gin.Engine, workbench *handler.PixLabWorkbenchHandler) {
	if workbench == nil {
		return
	}
	root := r.Group("/api/v1/pixlab-workbench")
	root.POST("/session", workbench.CreateSession)
	root.DELETE("/session", workbench.DeleteSession)

	projects := root.Group("/projects/:project_code", workbench.AuthenticateProject())
	projects.GET("/context", workbench.Context)
	projects.GET("/folders", workbench.ListFolders)
	projects.GET("/documents", workbench.ListDocuments)
	projects.POST("/documents", workbench.UploadDocument)
	projects.POST("/documents/status", workbench.DocumentStatuses)
	projects.GET("/documents/:document_id", workbench.GetDocument)
	projects.GET("/documents/:document_id/stages", workbench.DocumentStages)
	projects.GET("/documents/:document_id/preview", workbench.PreviewDocument)
	projects.POST("/documents/:document_id/reparse", workbench.ReparseDocument)
	projects.GET("/sessions", workbench.ListChatSessions)
	projects.POST("/sessions", workbench.CreateChatSession)
	projects.GET("/sessions/:session_id", workbench.GetChatSession)
	projects.DELETE("/sessions/:session_id", workbench.DeleteChatSession)
	projects.GET("/sessions/:session_id/messages", workbench.ChatMessages)
	projects.POST("/sessions/:session_id/answers", workbench.ChatAnswer)
	projects.POST("/sessions/:session_id/stop", workbench.StopChatAnswer)
	projects.GET("/sessions/:session_id/continue-stream", workbench.ContinueChatAnswer)
}
