package handler

import (
	"bytes"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	appservice "github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/filetransport"
	sessionhandler "github.com/Tencent/WeKnora/internal/handler/session"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

const (
	pixLabSessionContextKey = "PixLabWorkbenchSession"
	pixLabBindingContextKey = "PixLabWorkbenchBinding"
)

type PixLabWorkbenchHandler struct {
	service        *appservice.PixLabWorkbenchService
	sessions       interfaces.SessionService
	messages       interfaces.MessageService
	chunks         interfaces.ChunkService
	sessionHandler *sessionhandler.Handler
	spanRepo       repository.KnowledgeSpanRepository
}

func NewPixLabWorkbenchHandler(service *appservice.PixLabWorkbenchService) *PixLabWorkbenchHandler {
	return &PixLabWorkbenchHandler{service: service}
}

// NewPixLabWorkbenchHandlerWithDependencies is the production composition
// root. The small constructor above stays available for focused document tests.
func NewPixLabWorkbenchHandlerWithDependencies(
	service *appservice.PixLabWorkbenchService,
	sessions interfaces.SessionService,
	messages interfaces.MessageService,
	chunks interfaces.ChunkService,
	sessionHandler *sessionhandler.Handler,
	spanRepo repository.KnowledgeSpanRepository,
) *PixLabWorkbenchHandler {
	return &PixLabWorkbenchHandler{
		service: service, sessions: sessions, messages: messages, chunks: chunks,
		sessionHandler: sessionHandler, spanRepo: spanRepo,
	}
}

type pixLabSessionRequest struct {
	Ticket      string `json:"ticket" binding:"required"`
	ProjectCode string `json:"project_code" binding:"required"`
}

func (h *PixLabWorkbenchHandler) CreateSession(c *gin.Context) {
	if !h.requireOrigin(c) {
		return
	}
	var request pixLabSessionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.fail(c, appserviceError(http.StatusBadRequest, "INVALID_REQUEST", "Ticket and project_code are required", err))
		return
	}
	sessionToken, csrfToken, principal, _, err := h.service.ExchangeTicket(
		c.Request.Context(), request.Ticket, request.ProjectCode,
	)
	if err != nil {
		h.fail(c, err)
		return
	}
	config := h.service.Config()
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		appservice.PixLabWorkbenchCookieName,
		sessionToken,
		int(config.SessionTTL.Seconds()),
		"/api/v1/pixlab-workbench/",
		"",
		config.CookieSecure(),
		true,
	)
	h.success(c, http.StatusOK, gin.H{
		"project_code": principal.ProjectCode,
		"csrf_token":   csrfToken,
		"expires_in":   int(config.SessionTTL.Seconds()),
	})
}

func (h *PixLabWorkbenchHandler) DeleteSession(c *gin.Context) {
	if !h.requireOrigin(c) {
		return
	}
	token, _ := c.Cookie(appservice.PixLabWorkbenchCookieName)
	if err := h.service.DeleteAuthenticatedSession(c.Request.Context(), token, c.GetHeader("X-CSRF-Token")); err != nil {
		h.fail(c, err)
		return
	}
	config := h.service.Config()
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(appservice.PixLabWorkbenchCookieName, "", -1, "/api/v1/pixlab-workbench/", "", config.CookieSecure(), true)
	h.success(c, http.StatusOK, gin.H{"deleted": true})
}

func (h *PixLabWorkbenchHandler) AuthenticateProject() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.requireOrigin(c) {
			c.Abort()
			return
		}
		token, err := c.Cookie(appservice.PixLabWorkbenchCookieName)
		if err != nil {
			h.fail(c, appserviceError(http.StatusUnauthorized, "UNAUTHENTICATED", "Workbench session is missing", err))
			c.Abort()
			return
		}
		requireCSRF := c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions
		session, binding, scoped, err := h.service.Authenticate(
			c.Request.Context(), token, c.Param("project_code"), c.GetHeader("X-CSRF-Token"), requireCSRF,
		)
		if err != nil {
			h.fail(c, err)
			c.Abort()
			return
		}
		c.Set(pixLabSessionContextKey, session)
		c.Set(pixLabBindingContextKey, binding)
		c.Set(types.TenantIDContextKey.String(), binding.TenantID)
		c.Set(types.UserIDContextKey.String(), "pixlab:"+session.Principal.UserID)
		c.Request = c.Request.WithContext(scoped)
		c.Next()
	}
}

func (h *PixLabWorkbenchHandler) Context(c *gin.Context) {
	session, binding, ok := h.scope(c)
	if !ok {
		return
	}
	kb, err := h.service.KnowledgeBaseService().GetKnowledgeBaseByID(c.Request.Context(), binding.KnowledgeBaseID)
	if err != nil {
		h.fail(c, err)
		return
	}
	projectName := binding.ProjectName
	if session.Principal.ProjectName != "" {
		projectName = session.Principal.ProjectName
	}
	h.success(c, http.StatusOK, gin.H{
		"project_code":   binding.ProjectCode,
		"project_name":   projectName,
		"knowledge_base": gin.H{"id": kb.ID, "name": kb.Name},
		"agent_id":       binding.AgentID,
		"capabilities":   session.Principal.Capabilities,
		"readiness": gin.H{
			"parser": "ready", "retrieval": "ready", "chat": "ready",
		},
	})
}

func (h *PixLabWorkbenchHandler) ListFolders(c *gin.Context) {
	_, binding, ok := h.scope(c)
	if !ok {
		return
	}
	tree, err := h.service.KnowledgeService().ListKnowledgeFolderTree(c.Request.Context(), binding.KnowledgeBaseID)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.success(c, http.StatusOK, tree)
}

func (h *PixLabWorkbenchHandler) ListDocuments(c *gin.Context) {
	_, binding, ok := h.scope(c)
	if !ok {
		return
	}
	page := parseBoundedInt(c.Query("page"), 1, 1, 1_000_000)
	size := parseBoundedInt(c.Query("size"), 20, 1, 100)
	pagination := &types.Pagination{Page: page, PageSize: size}
	filter := types.KnowledgeListFilter{}
	if rawFolder, exists := c.GetQuery("folder_path"); exists {
		filter.FolderPath = types.NormalizeKnowledgeFolderPath(rawFolder)
		filter.FolderScope = types.FolderScopeExact
	} else if rawFolder, exists := c.GetQuery("folder"); exists {
		filter.FolderPath = types.NormalizeKnowledgeFolderPath(rawFolder)
		filter.FolderScope = types.FolderScopeExact
	}
	filter.Keyword = strings.TrimSpace(c.Query("query"))
	filter.ParseStatus = strings.TrimSpace(c.Query("status"))
	if rawSortBy := strings.TrimSpace(c.Query("sort_by")); rawSortBy != "" {
		filter.SortBy = types.KnowledgeListSortField(rawSortBy)
		if !filter.SortBy.Valid() {
			h.fail(c, appserviceError(http.StatusBadRequest, "INVALID_REQUEST", "sort_by is invalid", nil))
			return
		}
	}
	if rawSortOrder := strings.TrimSpace(c.Query("sort_order")); rawSortOrder != "" {
		filter.SortOrder = types.KnowledgeListSortOrder(rawSortOrder)
		if !filter.SortOrder.Valid() {
			h.fail(c, appserviceError(http.StatusBadRequest, "INVALID_REQUEST", "sort_order is invalid", nil))
			return
		}
	}
	result, err := h.service.KnowledgeService().ListPagedKnowledgeByKnowledgeBaseID(
		c.Request.Context(), binding.KnowledgeBaseID, pagination, filter,
	)
	if err != nil {
		h.fail(c, err)
		return
	}
	documents, ok := result.Data.([]*types.Knowledge)
	if !ok {
		h.fail(c, appserviceError(http.StatusInternalServerError, "WEKNORA_ERROR", "Document list has an invalid shape", nil))
		return
	}
	views := make([]pixLabDocumentView, 0, len(documents))
	for _, document := range documents {
		if document != nil {
			views = append(views, newPixLabDocumentView(document))
		}
	}
	h.success(c, http.StatusOK, gin.H{
		"total": result.Total, "page": result.Page,
		"page_size": result.PageSize, "documents": views,
	})
}

func (h *PixLabWorkbenchHandler) GetDocument(c *gin.Context) {
	_, binding, ok := h.scope(c)
	if !ok {
		return
	}
	document, err := h.service.KnowledgeService().GetKnowledgeByID(c.Request.Context(), c.Param("document_id"))
	if err != nil || document == nil || document.KnowledgeBaseID != binding.KnowledgeBaseID {
		h.fail(c, appserviceError(http.StatusNotFound, "DOCUMENT_NOT_FOUND", "Document was not found", err))
		return
	}
	h.success(c, http.StatusOK, newPixLabDocumentView(document))
}

type pixLabDocumentStatusRequest struct {
	IDs []string `json:"ids" binding:"required"`
}

func (h *PixLabWorkbenchHandler) DocumentStatuses(c *gin.Context) {
	_, binding, ok := h.scope(c)
	if !ok {
		return
	}
	var request pixLabDocumentStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.IDs) == 0 || len(request.IDs) > 100 {
		h.fail(c, appserviceError(http.StatusBadRequest, "INVALID_REQUEST", "ids must contain between 1 and 100 document IDs", err))
		return
	}
	seen := make(map[string]struct{}, len(request.IDs))
	views := make([]pixLabDocumentView, 0, len(request.IDs))
	for _, rawID := range request.IDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			h.fail(c, appserviceError(http.StatusBadRequest, "INVALID_REQUEST", "Document IDs cannot be empty", nil))
			return
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		document, err := h.service.KnowledgeService().GetKnowledgeByID(c.Request.Context(), id)
		if err != nil || document == nil || document.KnowledgeBaseID != binding.KnowledgeBaseID {
			h.fail(c, appserviceError(http.StatusNotFound, "DOCUMENT_NOT_FOUND", "Document was not found", err))
			return
		}
		views = append(views, newPixLabDocumentView(document))
	}
	h.success(c, http.StatusOK, gin.H{"documents": views})
}

func (h *PixLabWorkbenchHandler) DocumentStages(c *gin.Context) {
	_, binding, ok := h.scope(c)
	if !ok {
		return
	}
	document, err := h.service.KnowledgeService().GetKnowledgeByID(c.Request.Context(), c.Param("document_id"))
	if err != nil || document == nil || document.KnowledgeBaseID != binding.KnowledgeBaseID {
		h.fail(c, appserviceError(http.StatusNotFound, "DOCUMENT_NOT_FOUND", "Document was not found", err))
		return
	}
	requestedAttempt := parseBoundedInt(c.Query("attempt"), 0, 0, 1_000_000)
	latestAttempt, currentAttempt := 0, 0
	rows := []types.KnowledgeProcessingSpan{}
	if h.spanRepo != nil {
		latestAttempt, err = h.spanRepo.LatestAttempt(c.Request.Context(), document.ID)
		if err != nil {
			h.fail(c, err)
			return
		}
		currentAttempt = latestAttempt
		if requestedAttempt > 0 {
			currentAttempt = requestedAttempt
		}
		if currentAttempt > 0 {
			rows, err = h.spanRepo.ListByAttempt(c.Request.Context(), document.ID, currentAttempt)
			if err != nil {
				h.fail(c, err)
				return
			}
		}
	}
	tree, currentStage, lastFailure := buildSpanTree(document.ID, currentAttempt, rows, document.ParseStatus)
	response := gin.H{
		"document_id": document.ID, "attempt": currentAttempt,
		"latest_attempt": latestAttempt, "parse_status": document.ParseStatus,
		"current_stage": currentStage, "trace": tree,
	}
	if lastFailure != nil {
		response["last_error"] = gin.H{
			"code": lastFailure.ErrorCode, "message": lastFailure.ErrorMessage,
			"stage": lastFailure.Name, "occurred_at": lastFailure.UpdatedAt,
		}
	} else if document.ErrorMessage != "" {
		response["last_error"] = gin.H{"message": document.ErrorMessage, "occurred_at": document.UpdatedAt}
	}
	h.success(c, http.StatusOK, response)
}

func (h *PixLabWorkbenchHandler) PreviewDocument(c *gin.Context) {
	_, binding, ok := h.scope(c)
	if !ok {
		return
	}
	document, err := h.service.KnowledgeService().GetKnowledgeByID(c.Request.Context(), c.Param("document_id"))
	if err != nil || document == nil || document.KnowledgeBaseID != binding.KnowledgeBaseID {
		h.fail(c, appserviceError(http.StatusNotFound, "DOCUMENT_NOT_FOUND", "Document was not found", err))
		return
	}
	file, filename, err := h.service.KnowledgeService().GetKnowledgeFile(c.Request.Context(), document.ID)
	if err != nil {
		h.fail(c, appserviceError(http.StatusInternalServerError, "PREVIEW_FAILED", "Failed to retrieve document preview", err))
		return
	}
	if err := filetransport.Serve(c.Writer, c.Request, file, filetransport.Options{
		Filename: filename, CacheControl: "private, no-store",
	}); err != nil {
		logger.Errorf(c.Request.Context(), "PixLab workbench preview failed: %v", err)
	}
}

func (h *PixLabWorkbenchHandler) UploadDocument(c *gin.Context) {
	session, binding, ok := h.scope(c)
	if !ok {
		return
	}
	if !session.Principal.HasCapability(types.PixLabCapabilityUpload) {
		h.fail(c, appserviceError(http.StatusForbidden, "PROJECT_FORBIDDEN", "Principal cannot upload to this project", nil))
		return
	}
	maxSizeMB := utils.GetMaxFileSizeMB()
	maxSize := maxSizeMB * 1024 * 1024
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize+1024*1024)
	file, err := c.FormFile("file")
	if err != nil {
		h.fail(c, appserviceError(http.StatusBadRequest, "UPLOAD_FAILED", "File upload failed", err))
		return
	}
	if file.Size > maxSize {
		h.fail(c, appserviceError(http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", fmt.Sprintf("File size cannot exceed %d MB", maxSizeMB), nil))
		return
	}
	customFileName := strings.TrimSpace(c.PostForm("fileName"))
	if customFileName == "" {
		customFileName = file.Filename
	}
	if err := validateWorkbenchRelativeFileName(customFileName); err != nil {
		h.fail(c, appserviceError(http.StatusBadRequest, "INVALID_FILE_NAME", "File name must be a safe relative path", err))
		return
	}
	metadata := map[string]string{}
	if raw := c.PostForm("metadata"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			h.fail(c, appserviceError(http.StatusBadRequest, "INVALID_METADATA", "Metadata must be a string map", err))
			return
		}
	}
	metadata["pixlab_project_code"] = binding.ProjectCode
	metadata["pixlab_uploader_user_id"] = session.Principal.UserID
	metadata["source_principal"] = "pixlab:" + session.Principal.UserID

	var processOverrides *types.KnowledgeProcessOverrides
	if raw := c.PostForm("process_config"); raw != "" {
		processOverrides = &types.KnowledgeProcessOverrides{}
		if err := json.Unmarshal([]byte(raw), processOverrides); err != nil {
			h.fail(c, appserviceError(http.StatusBadRequest, "INVALID_PROCESS_CONFIG", "Process configuration is invalid", err))
			return
		}
	}
	var enableMultimodel *bool
	if raw := c.PostForm("enable_multimodel"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			h.fail(c, appserviceError(http.StatusBadRequest, "INVALID_PROCESS_CONFIG", "enable_multimodel is invalid", err))
			return
		}
		enableMultimodel = &value
	}
	if err := appservice.ValidateJSONUploadContent(path.Base(customFileName), file); err != nil {
		h.fail(c, err)
		return
	}
	document, err := h.service.KnowledgeService().CreateKnowledgeFromFile(
		c.Request.Context(), binding.KnowledgeBaseID, file, metadata, enableMultimodel,
		customFileName, nil, "pixlab-workbench", processOverrides,
	)
	if err != nil {
		h.fail(c, mapDocumentError(err))
		return
	}
	h.success(c, http.StatusCreated, gin.H{
		"id": document.ID, "folder_path": document.FolderPath,
		"file_name": document.FileName, "parse_status": document.ParseStatus,
	})
}

func (h *PixLabWorkbenchHandler) ReparseDocument(c *gin.Context) {
	session, binding, ok := h.scope(c)
	if !ok {
		return
	}
	if !session.Principal.HasCapability(types.PixLabCapabilityUpload) {
		h.fail(c, appserviceError(http.StatusForbidden, "PROJECT_FORBIDDEN", "Principal cannot retry project documents", nil))
		return
	}
	document, err := h.service.KnowledgeService().GetKnowledgeByID(c.Request.Context(), c.Param("document_id"))
	if err != nil || document == nil || document.KnowledgeBaseID != binding.KnowledgeBaseID {
		h.fail(c, appserviceError(http.StatusNotFound, "DOCUMENT_NOT_FOUND", "Document was not found", err))
		return
	}
	metadata := document.GetMetadata()
	if metadata["pixlab_uploader_user_id"] != session.Principal.UserID {
		h.fail(c, appserviceError(http.StatusForbidden, "PROJECT_FORBIDDEN", "Only the uploader can retry this document", nil))
		return
	}
	if document.ParseStatus != types.ParseStatusFailed && document.ParseStatus != types.ParseStatusCancelled {
		h.fail(c, appserviceError(http.StatusConflict, "DOCUMENT_NOT_RETRYABLE", "Document is not in a retryable state", nil))
		return
	}
	var request struct {
		ProcessConfig *types.KnowledgeProcessOverrides `json:"process_config"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			h.fail(c, appserviceError(http.StatusBadRequest, "INVALID_PROCESS_CONFIG", "Process configuration is invalid", err))
			return
		}
	}
	document, err = h.service.KnowledgeService().ReparseKnowledge(c.Request.Context(), document.ID, request.ProcessConfig)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.success(c, http.StatusAccepted, gin.H{"id": document.ID, "parse_status": document.ParseStatus})
}

type pixLabChunkView struct {
	ID              string               `json:"id"`
	KnowledgeID     string               `json:"knowledge_id"`
	KnowledgeBaseID string               `json:"knowledge_base_id"`
	Content         string               `json:"content"`
	ChunkIndex      int                  `json:"chunk_index"`
	ChunkType       types.ChunkType      `json:"chunk_type"`
	ParentChunkID   string               `json:"parent_chunk_id,omitempty"`
	ContentRevision int                  `json:"content_revision"`
	SourceLocators  types.SourceLocators `json:"source_locators,omitempty"`
	ImageInfo       string               `json:"image_info,omitempty"`
}

func newPixLabChunkView(chunk *types.Chunk) pixLabChunkView {
	return pixLabChunkView{
		ID: chunk.ID, KnowledgeID: chunk.KnowledgeID, KnowledgeBaseID: chunk.KnowledgeBaseID,
		Content: chunk.Content, ChunkIndex: chunk.ChunkIndex, ChunkType: chunk.ChunkType,
		ParentChunkID: chunk.ParentChunkID, ContentRevision: chunk.ContentRevision,
		SourceLocators: chunk.SourceLocators, ImageInfo: chunk.ImageInfo,
	}
}

func (h *PixLabWorkbenchHandler) ListDocumentChunks(c *gin.Context) {
	_, binding, ok := h.scope(c)
	if !ok || !h.requireChunkService(c) {
		return
	}
	documentID := c.Param("document_id")
	document, err := h.service.KnowledgeService().GetKnowledgeByID(c.Request.Context(), documentID)
	if err != nil || document == nil || document.KnowledgeBaseID != binding.KnowledgeBaseID {
		h.fail(c, appserviceError(http.StatusNotFound, "DOCUMENT_NOT_FOUND", "Document was not found", err))
		return
	}
	pagination := &types.Pagination{
		Page:     parseBoundedInt(c.Query("page"), 1, 1, 1_000_000),
		PageSize: parseBoundedInt(c.Query("size"), 25, 1, 100),
	}
	chunkTypes := []types.ChunkType{
		types.ChunkTypeText, types.ChunkTypeParentText, types.ChunkTypeImageOCR,
		types.ChunkTypeImageCaption, types.ChunkTypeSummary, types.ChunkTypeTableSummary,
		types.ChunkTypeTableColumn,
	}
	result, err := h.chunks.ListPagedChunksByKnowledgeID(c.Request.Context(), documentID, pagination, chunkTypes)
	if err != nil {
		h.fail(c, err)
		return
	}
	chunks, valid := result.Data.([]*types.Chunk)
	if !valid {
		h.fail(c, appserviceError(http.StatusInternalServerError, "WEKNORA_ERROR", "Chunk list has an invalid shape", nil))
		return
	}
	views := make([]pixLabChunkView, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk == nil || chunk.KnowledgeBaseID != binding.KnowledgeBaseID || chunk.KnowledgeID != documentID {
			continue
		}
		views = append(views, newPixLabChunkView(chunk))
	}
	h.success(c, http.StatusOK, gin.H{
		"chunks": views, "total": result.Total, "page": result.Page, "page_size": result.PageSize,
	})
}

func (h *PixLabWorkbenchHandler) GetDocumentChunk(c *gin.Context) {
	h.getProjectChunk(c, c.Param("document_id"))
}

func (h *PixLabWorkbenchHandler) GetProjectChunk(c *gin.Context) {
	h.getProjectChunk(c, "")
}

func (h *PixLabWorkbenchHandler) getProjectChunk(c *gin.Context, documentID string) {
	_, binding, ok := h.scope(c)
	if !ok || !h.requireChunkService(c) {
		return
	}
	chunk, err := h.chunks.GetChunkByID(c.Request.Context(), c.Param("chunk_id"))
	if err != nil || chunk == nil || chunk.KnowledgeBaseID != binding.KnowledgeBaseID ||
		(documentID != "" && chunk.KnowledgeID != documentID) {
		h.fail(c, appserviceError(http.StatusNotFound, "CHUNK_NOT_FOUND", "Chunk was not found", err))
		return
	}
	h.success(c, http.StatusOK, newPixLabChunkView(chunk))
}

func (h *PixLabWorkbenchHandler) requireChunkService(c *gin.Context) bool {
	if h.chunks != nil {
		return true
	}
	h.fail(c, appserviceError(http.StatusServiceUnavailable, "WEKNORA_ERROR", "Chunk service is unavailable", nil))
	return false
}

type pixLabChatSessionView struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func newPixLabChatSessionView(value *types.Session) pixLabChatSessionView {
	return pixLabChatSessionView{
		ID: value.ID, Title: value.Title, Description: value.Description,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func (h *PixLabWorkbenchHandler) ListChatSessions(c *gin.Context) {
	workbenchSession, binding, ok := h.scope(c)
	if !ok || !h.requireChatServices(c) {
		return
	}
	if !workbenchSession.Principal.HasCapability(types.PixLabCapabilityChat) {
		h.fail(c, appserviceError(http.StatusForbidden, "PROJECT_FORBIDDEN", "Principal cannot chat in this project", nil))
		return
	}
	rows, err := h.sessions.GetSessionsByTenant(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	filtered := make([]*types.Session, 0, len(rows))
	for _, row := range rows {
		if row != nil && row.TenantID == binding.TenantID && row.PixLabProjectCode != nil &&
			*row.PixLabProjectCode == binding.ProjectCode {
			filtered = append(filtered, row)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt) })
	page := parseBoundedInt(c.Query("page"), 1, 1, 1_000_000)
	size := parseBoundedInt(c.Query("size"), 20, 1, 100)
	start := (page - 1) * size
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + size
	if end > len(filtered) {
		end = len(filtered)
	}
	views := make([]pixLabChatSessionView, 0, end-start)
	for _, row := range filtered[start:end] {
		views = append(views, newPixLabChatSessionView(row))
	}
	h.success(c, http.StatusOK, gin.H{
		"sessions": views, "total": len(filtered), "page": page, "page_size": size,
	})
}

func (h *PixLabWorkbenchHandler) CreateChatSession(c *gin.Context) {
	workbenchSession, binding, ok := h.scope(c)
	if !ok || !h.requireChatServices(c) {
		return
	}
	if !workbenchSession.Principal.HasCapability(types.PixLabCapabilityChat) {
		h.fail(c, appserviceError(http.StatusForbidden, "PROJECT_FORBIDDEN", "Principal cannot chat in this project", nil))
		return
	}
	var request struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			h.fail(c, appserviceError(http.StatusBadRequest, "INVALID_REQUEST", "Session request is invalid", err))
			return
		}
	}
	request.Title = strings.TrimSpace(request.Title)
	if len(request.Title) > 255 || len(request.Description) > 2_000 {
		h.fail(c, appserviceError(http.StatusBadRequest, "INVALID_REQUEST", "Session title or description is too long", nil))
		return
	}
	projectCode := binding.ProjectCode
	created, err := h.sessions.CreateSession(c.Request.Context(), &types.Session{
		TenantID: binding.TenantID, UserID: "pixlab:" + workbenchSession.Principal.UserID,
		PixLabProjectCode: &projectCode, Title: request.Title,
		Description: types.SanitizeClientSessionDescription(request.Description, ""),
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	h.success(c, http.StatusCreated, newPixLabChatSessionView(created))
}

func (h *PixLabWorkbenchHandler) GetChatSession(c *gin.Context) {
	if _, _, ok := h.scope(c); !ok || !h.requireChatServices(c) {
		return
	}
	session, ok := h.projectChatSession(c, c.Param("session_id"))
	if !ok {
		return
	}
	h.success(c, http.StatusOK, newPixLabChatSessionView(session))
}

func (h *PixLabWorkbenchHandler) DeleteChatSession(c *gin.Context) {
	if _, _, ok := h.scope(c); !ok || !h.requireChatServices(c) {
		return
	}
	session, ok := h.projectChatSession(c, c.Param("session_id"))
	if !ok {
		return
	}
	if err := h.sessions.DeleteSession(c.Request.Context(), session.ID); err != nil {
		h.fail(c, err)
		return
	}
	h.success(c, http.StatusOK, gin.H{"deleted": true})
}

func (h *PixLabWorkbenchHandler) ChatMessages(c *gin.Context) {
	_, binding, ok := h.scope(c)
	if !ok || !h.requireChatServices(c) {
		return
	}
	if _, ok := h.projectChatSession(c, c.Param("session_id")); !ok {
		return
	}
	limit := parseBoundedInt(c.Query("limit"), 50, 1, 100)
	var (
		rows []*types.Message
		err  error
	)
	if rawBefore := strings.TrimSpace(c.Query("before_time")); rawBefore != "" {
		before, parseErr := time.Parse(time.RFC3339Nano, rawBefore)
		if parseErr != nil {
			before, parseErr = time.Parse(time.RFC3339, rawBefore)
		}
		if parseErr != nil {
			h.fail(c, appserviceError(http.StatusBadRequest, "INVALID_REQUEST", "before_time must be RFC3339", parseErr))
			return
		}
		rows, err = h.messages.GetMessagesBySessionBeforeTime(c.Request.Context(), c.Param("session_id"), before, limit)
	} else {
		rows, err = h.messages.GetRecentMessagesBySession(c.Request.Context(), c.Param("session_id"), limit)
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	views := make([]pixLabMessageView, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			views = append(views, h.newPixLabMessageView(c, binding, row))
		}
	}
	h.success(c, http.StatusOK, gin.H{"messages": views})
}

func (h *PixLabWorkbenchHandler) ChatAnswer(c *gin.Context) {
	workbenchSession, binding, ok := h.scope(c)
	if !ok || !h.requireChatServices(c) {
		return
	}
	if !workbenchSession.Principal.HasCapability(types.PixLabCapabilityChat) {
		h.fail(c, appserviceError(http.StatusForbidden, "PROJECT_FORBIDDEN", "Principal cannot chat in this project", nil))
		return
	}
	if _, ok := h.projectChatSession(c, c.Param("session_id")); !ok {
		return
	}
	var request struct {
		Query           string `json:"query" binding:"required"`
		ReasoningEffort string `json:"reasoning_effort,omitempty"`
		DisableTitle    bool   `json:"disable_title,omitempty"`
	}
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 256*1024))
	if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.Query) == "" {
		h.fail(c, appserviceError(http.StatusBadRequest, "INVALID_REQUEST", "A non-empty query is required", err))
		return
	}
	forward := sessionhandler.CreateKnowledgeQARequest{
		Query: request.Query, KnowledgeBaseIDs: []string{binding.KnowledgeBaseID},
		AgentID: binding.AgentID, AgentEnabled: true, Channel: "pixlab-workbench",
		ReasoningEffort: request.ReasoningEffort, DisableTitle: request.DisableTitle,
	}
	body, err := json.Marshal(forward)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.ContentLength = int64(len(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.URL.RawQuery = withoutQueryParameter(c.Request.URL.Query(), "resource_urls").Encode()
	h.sessionHandler.KnowledgeQA(c)
}

func (h *PixLabWorkbenchHandler) StopChatAnswer(c *gin.Context) {
	if _, _, ok := h.scope(c); !ok || !h.requireChatServices(c) {
		return
	}
	if _, ok := h.projectChatSession(c, c.Param("session_id")); !ok {
		return
	}
	h.sessionHandler.StopSession(c)
}

func (h *PixLabWorkbenchHandler) ContinueChatAnswer(c *gin.Context) {
	if _, _, ok := h.scope(c); !ok || !h.requireChatServices(c) {
		return
	}
	if _, ok := h.projectChatSession(c, c.Param("session_id")); !ok {
		return
	}
	c.Request.URL.RawQuery = withoutQueryParameter(c.Request.URL.Query(), "resource_urls").Encode()
	h.sessionHandler.ContinueStream(c)
}

func (h *PixLabWorkbenchHandler) projectChatSession(c *gin.Context, id string) (*types.Session, bool) {
	_, binding, ok := h.scope(c)
	if !ok {
		return nil, false
	}
	value, err := h.sessions.GetOwnedSession(c.Request.Context(), strings.TrimSpace(id))
	if err != nil || value == nil || value.TenantID != binding.TenantID ||
		value.PixLabProjectCode == nil || *value.PixLabProjectCode != binding.ProjectCode {
		h.fail(c, appserviceError(http.StatusNotFound, "SESSION_SCOPE_MISMATCH", "Session was not found", err))
		return nil, false
	}
	return value, true
}

func (h *PixLabWorkbenchHandler) requireChatServices(c *gin.Context) bool {
	if h.sessions == nil || h.messages == nil || h.sessionHandler == nil {
		h.fail(c, appserviceError(http.StatusServiceUnavailable, "CHAT_UNAVAILABLE", "Workbench chat is unavailable", nil))
		return false
	}
	return true
}

type pixLabCitationView struct {
	ID               string               `json:"id"`
	DocumentID       string               `json:"document_id"`
	DocumentTitle    string               `json:"document_title"`
	DocumentFileName string               `json:"document_file_name"`
	Content          string               `json:"content"`
	ChunkIndex       int                  `json:"chunk_index"`
	StartAt          int                  `json:"start_at"`
	EndAt            int                  `json:"end_at"`
	SourceLocators   types.SourceLocators `json:"source_locators,omitempty"`
}

type pixLabMessageView struct {
	ID        string               `json:"id"`
	Role      string               `json:"role"`
	Content   string               `json:"content"`
	Completed bool                 `json:"completed"`
	Fallback  bool                 `json:"fallback,omitempty"`
	Citations []pixLabCitationView `json:"citations"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
}

func (h *PixLabWorkbenchHandler) newPixLabMessageView(
	c *gin.Context,
	binding *types.PixLabProjectBinding,
	message *types.Message,
) pixLabMessageView {
	citations := make([]pixLabCitationView, 0, len(message.KnowledgeReferences))
	documentCache := map[string]*types.Knowledge{}
	for _, reference := range message.KnowledgeReferences {
		if reference == nil || reference.KnowledgeID == "" {
			continue
		}
		allowed := reference.KnowledgeBaseID == binding.KnowledgeBaseID
		var document *types.Knowledge
		if cached, exists := documentCache[reference.KnowledgeID]; exists {
			document = cached
		} else {
			document, _ = h.service.KnowledgeService().GetKnowledgeByID(c.Request.Context(), reference.KnowledgeID)
			documentCache[reference.KnowledgeID] = document
		}
		if document != nil {
			allowed = document.KnowledgeBaseID == binding.KnowledgeBaseID
		}
		if !allowed {
			continue
		}
		fileName := reference.KnowledgeFilename
		if document != nil && fileName == "" {
			fileName = document.FileName
		}
		citations = append(citations, pixLabCitationView{
			ID: reference.ID, DocumentID: reference.KnowledgeID,
			DocumentTitle: reference.KnowledgeTitle, DocumentFileName: fileName,
			Content: reference.Content, ChunkIndex: reference.ChunkIndex,
			StartAt: reference.StartAt, EndAt: reference.EndAt,
			SourceLocators: reference.SourceLocators,
		})
	}
	return pixLabMessageView{
		ID: message.ID, Role: message.Role, Content: message.Content,
		Completed: message.IsCompleted, Fallback: message.IsFallback,
		Citations: citations, CreatedAt: message.CreatedAt, UpdatedAt: message.UpdatedAt,
	}
}

func withoutQueryParameter(values url.Values, name string) url.Values {
	cleaned := make(url.Values, len(values))
	for key, value := range values {
		if key != name {
			cleaned[key] = append([]string(nil), value...)
		}
	}
	return cleaned
}

func (h *PixLabWorkbenchHandler) scope(c *gin.Context) (*types.PixLabWorkbenchSession, *types.PixLabProjectBinding, bool) {
	sessionValue, sessionOK := c.Get(pixLabSessionContextKey)
	bindingValue, bindingOK := c.Get(pixLabBindingContextKey)
	session, sessionTypeOK := sessionValue.(*types.PixLabWorkbenchSession)
	binding, bindingTypeOK := bindingValue.(*types.PixLabProjectBinding)
	if !sessionOK || !bindingOK || !sessionTypeOK || !bindingTypeOK {
		h.fail(c, appserviceError(http.StatusUnauthorized, "UNAUTHENTICATED", "Workbench authorization is missing", nil))
		return nil, nil, false
	}
	return session, binding, true
}

func (h *PixLabWorkbenchHandler) requireOrigin(c *gin.Context) bool {
	if err := h.service.RequireEnabled(); err != nil {
		h.fail(c, err)
		return false
	}
	expected := h.service.Config().PublicOrigin
	if expected == "" {
		h.fail(c, appserviceError(http.StatusServiceUnavailable, "BACKCHANNEL_NOT_CONFIGURED", "PixLab public origin is not configured", nil))
		return false
	}
	origin := strings.TrimRight(strings.TrimSpace(c.GetHeader("Origin")), "/")
	safeMethod := c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions
	if origin == "" && safeMethod {
		fetchSite := strings.TrimSpace(c.GetHeader("Sec-Fetch-Site"))
		if fetchSite == "" || fetchSite == "same-origin" {
			return true
		}
	}
	if origin == "" || !hmac.Equal([]byte(origin), []byte(expected)) {
		h.fail(c, appserviceError(http.StatusForbidden, "ORIGIN_FORBIDDEN", "Request origin is not allowed", nil))
		return false
	}
	return true
}

type pixLabTagView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type pixLabDocumentView struct {
	ID                   string          `json:"id"`
	Title                string          `json:"title"`
	Description          string          `json:"description"`
	FileName             string          `json:"file_name"`
	FolderPath           string          `json:"folder_path"`
	FileType             string          `json:"file_type"`
	FileSize             int64           `json:"file_size"`
	ParseStatus          string          `json:"parse_status"`
	SummaryStatus        string          `json:"summary_status"`
	PendingSubtasksCount int             `json:"pending_subtasks_count"`
	ErrorMessage         string          `json:"error_message,omitempty"`
	Tags                 []pixLabTagView `json:"tags"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
	ProcessedAt          *time.Time      `json:"processed_at,omitempty"`
	LastActivityAt       *time.Time      `json:"last_activity_at,omitempty"`
	StallState           string          `json:"stall_state,omitempty"`
}

func newPixLabDocumentView(document *types.Knowledge) pixLabDocumentView {
	tags := make([]pixLabTagView, 0, len(document.Tags))
	for _, tag := range document.Tags {
		if tag != nil {
			tags = append(tags, pixLabTagView{ID: tag.ID, Name: tag.Name, Color: tag.Color})
		}
	}
	return pixLabDocumentView{
		ID: document.ID, Title: document.Title, Description: document.Description,
		FileName: document.FileName, FolderPath: document.FolderPath,
		FileType: document.FileType, FileSize: document.FileSize,
		ParseStatus: document.ParseStatus, SummaryStatus: document.SummaryStatus,
		PendingSubtasksCount: document.PendingSubtasksCount,
		ErrorMessage:         document.ErrorMessage, Tags: tags,
		CreatedAt: document.CreatedAt, UpdatedAt: document.UpdatedAt,
		ProcessedAt: document.ProcessedAt, LastActivityAt: document.LastActivityAt,
		StallState: document.StallState,
	}
}

func (h *PixLabWorkbenchHandler) success(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data, "request_id": c.GetString(types.RequestIDContextKey.String())})
}

func (h *PixLabWorkbenchHandler) fail(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, "WEKNORA_ERROR", "Workbench request failed"
	var workbenchErr *appservice.PixLabWorkbenchError
	if errors.As(err, &workbenchErr) {
		status, code, message = workbenchErr.Status, workbenchErr.Code, workbenchErr.Message
	} else if appErr, ok := apperrors.IsAppError(err); ok {
		status, code, message = appErr.HTTPCode, strconv.Itoa(int(appErr.Code)), appErr.Message
	}
	if status >= 500 {
		logger.Errorf(c.Request.Context(), "PixLab workbench request failed: %v", err)
	}
	c.AbortWithStatusJSON(status, gin.H{
		"code": code, "message": message,
		"request_id": c.GetString(types.RequestIDContextKey.String()),
	})
}

func appserviceError(status int, code, message string, cause error) error {
	return &appservice.PixLabWorkbenchError{Status: status, Code: code, Message: message, Cause: cause}
}

func mapDocumentError(err error) error {
	var duplicate *types.DuplicateKnowledgeError
	if errors.As(err, &duplicate) {
		return appserviceError(http.StatusConflict, "DUPLICATE_FILE", "A document with the same content already exists", err)
	}
	if errors.Is(err, appservice.ErrInvalidFileType) {
		return appserviceError(http.StatusBadRequest, "UNSUPPORTED_FILE", "File type is not supported", err)
	}
	return err
}

func validateWorkbenchRelativeFileName(value string) error {
	if value == "" || strings.ContainsRune(value, '\x00') {
		return errors.New("empty or invalid filename")
	}
	normalized := strings.ReplaceAll(value, "\\", "/")
	if strings.HasPrefix(normalized, "/") || path.IsAbs(normalized) || strings.Contains(normalized, ":/") {
		return errors.New("absolute path is not allowed")
	}
	segments := strings.Split(normalized, "/")
	for _, segment := range segments {
		if strings.TrimSpace(segment) == "" || segment == "." || segment == ".." {
			return errors.New("empty or traversal path segment")
		}
	}
	_, fileName := types.SplitKnowledgeRelativePath(normalized)
	if fileName == "" || strings.Contains(fileName, "..") {
		return errors.New("invalid base filename")
	}
	return nil
}

func parseBoundedInt(raw string, fallback, minimum, maximum int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return fallback
	}
	return value
}
