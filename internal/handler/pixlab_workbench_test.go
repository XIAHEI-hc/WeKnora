package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"sync"
	"testing"
	"time"

	appservice "github.com/Tencent/WeKnora/internal/application/service"
	sessionhandler "github.com/Tencent/WeKnora/internal/handler/session"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type handlerWorkbenchRepo struct {
	binding *types.PixLabProjectBinding
}

func (r *handlerWorkbenchRepo) GetProjectBinding(_ context.Context, _ string) (*types.PixLabProjectBinding, error) {
	copy := *r.binding
	return &copy, nil
}

type handlerWorkbenchKB struct {
	interfaces.KnowledgeBaseService
	kb *types.KnowledgeBase
}

func (s *handlerWorkbenchKB) GetKnowledgeBaseByID(_ context.Context, _ string) (*types.KnowledgeBase, error) {
	copy := *s.kb
	return &copy, nil
}

type handlerWorkbenchKnowledge struct {
	interfaces.KnowledgeService
	testT        *testing.T
	createCalls  int
	kbID         string
	metadata     map[string]string
	userID       string
	tenantID     uint64
	tenantInfoID uint64
	documents    map[string]*types.Knowledge
	reparseCalls int
}

func (s *handlerWorkbenchKnowledge) ReparseKnowledge(
	_ context.Context,
	id string,
	_ *types.KnowledgeProcessOverrides,
) (*types.Knowledge, error) {
	document := s.documents[id]
	if document == nil {
		return nil, errors.New("knowledge not found")
	}
	s.reparseCalls++
	copy := *document
	copy.ParseStatus = types.ParseStatusPending
	return &copy, nil
}

func (s *handlerWorkbenchKnowledge) GetKnowledgeByID(_ context.Context, id string) (*types.Knowledge, error) {
	document := s.documents[id]
	if document == nil {
		return nil, errors.New("knowledge not found")
	}
	copy := *document
	return &copy, nil
}

func (s *handlerWorkbenchKnowledge) CreateKnowledgeFromFile(
	ctx context.Context,
	kbID string,
	file *multipart.FileHeader,
	metadata map[string]string,
	_ *bool,
	customFileName string,
	_ []string,
	channel string,
	_ *types.KnowledgeProcessOverrides,
) (*types.Knowledge, error) {
	s.createCalls++
	s.kbID = kbID
	s.metadata = metadata
	s.tenantID, _ = types.TenantIDFromContext(ctx)
	if tenant, ok := ctx.Value(types.TenantInfoContextKey).(*types.Tenant); ok && tenant != nil {
		s.tenantInfoID = tenant.ID
	}
	s.userID, _ = types.UserIDFromContext(ctx)
	folder, name := types.SplitKnowledgeRelativePath(customFileName)
	require.Equal(s.testT, "pixlab-workbench", channel)
	return &types.Knowledge{
		ID: "knowledge-1", KnowledgeBaseID: kbID, FileName: name,
		FolderPath: folder, ParseStatus: types.ParseStatusPending,
	}, nil
}

type handlerWorkbenchTenantRepo struct {
	interfaces.TenantRepository
	tenant *types.Tenant
}

func (r *handlerWorkbenchTenantRepo) GetTenantByID(_ context.Context, _ uint64) (*types.Tenant, error) {
	copy := *r.tenant
	return &copy, nil
}

func (s *handlerWorkbenchKnowledge) withTest(t *testing.T) *handlerWorkbenchKnowledge {
	s.testT = t
	return s
}

type handlerPrincipalClient struct {
	mu          sync.RWMutex
	principal   types.PixLabPrincipal
	validateErr error
}

func (c *handlerPrincipalClient) Redeem(_ context.Context, _, _ string) (*types.PixLabPrincipal, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	copy := c.principal
	return &copy, nil
}

func (c *handlerPrincipalClient) Validate(_ context.Context, _ types.PixLabPrincipal) (*types.PixLabPrincipal, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.validateErr != nil {
		return nil, c.validateErr
	}
	copy := c.principal
	return &copy, nil
}

func (c *handlerPrincipalClient) failValidation(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.validateErr = err
}

type workbenchHandlerFixture struct {
	handler         *PixLabWorkbenchHandler
	knowledge       *handlerWorkbenchKnowledge
	sessionToken    string
	csrfToken       string
	redis           *redis.Client
	principalClient *handlerPrincipalClient
}

type handlerSessionService struct {
	interfaces.SessionService
	sessions []*types.Session
}

func (s *handlerSessionService) GetOwnedSession(ctx context.Context, id string) (*types.Session, error) {
	owner, _ := types.UserIDFromContext(ctx)
	for _, session := range s.sessions {
		if session != nil && session.ID == id && session.UserID == owner {
			copy := *session
			return &copy, nil
		}
	}
	return nil, errors.New("session not found")
}

func (s *handlerSessionService) GetSessionsByTenant(ctx context.Context) ([]*types.Session, error) {
	owner, _ := types.UserIDFromContext(ctx)
	result := make([]*types.Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		if session != nil && session.UserID == owner {
			copy := *session
			result = append(result, &copy)
		}
	}
	return result, nil
}

type handlerMessageService struct {
	interfaces.MessageService
	messages map[string][]*types.Message
	calls    int
}

type handlerChunkService struct {
	interfaces.ChunkService
	chunks    map[string]*types.Chunk
	list      []*types.Chunk
	getCalls  int
	listCalls int
}

func (s *handlerChunkService) GetChunkByID(_ context.Context, id string) (*types.Chunk, error) {
	s.getCalls++
	chunk := s.chunks[id]
	if chunk == nil {
		return nil, errors.New("chunk not found")
	}
	copy := *chunk
	return &copy, nil
}

func (s *handlerChunkService) ListPagedChunksByKnowledgeID(
	_ context.Context,
	_ string,
	page *types.Pagination,
	_ []types.ChunkType,
) (*types.PageResult, error) {
	s.listCalls++
	rows := make([]*types.Chunk, 0, len(s.list))
	for _, chunk := range s.list {
		if chunk == nil {
			rows = append(rows, nil)
			continue
		}
		copy := *chunk
		rows = append(rows, &copy)
	}
	return types.NewPageResult(int64(len(rows)), page, rows), nil
}

func (s *handlerMessageService) GetRecentMessagesBySession(
	_ context.Context, sessionID string, _ int,
) ([]*types.Message, error) {
	s.calls++
	return s.messages[sessionID], nil
}

func newWorkbenchHandlerFixture(t *testing.T) *workbenchHandlerFixture {
	t.Helper()
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	binding := &types.PixLabProjectBinding{
		ProjectCode: "PROJECT_P", ProjectName: "Project P", TenantID: 10000,
		KnowledgeBaseID: "kb-p", AgentID: "agent-p", Status: types.PixLabBindingStatusActive, Revision: 3,
	}
	knowledge := (&handlerWorkbenchKnowledge{}).withTest(t)
	principalClient := &handlerPrincipalClient{principal: types.PixLabPrincipal{
		UserID: "pixlab-user", ProjectCode: "PROJECT_P", PixLabSessionID: "pixlab-session",
		PermissionVersion: 2, BindingRevision: 3,
		Capabilities: []string{types.PixLabCapabilityRead, types.PixLabCapabilityUpload, types.PixLabCapabilityChat},
	}}
	service := appservice.NewPixLabWorkbenchServiceWithConfig(
		appservice.PixLabWorkbenchConfig{
			Enabled: true, InternalBaseURL: "http://pixlab.test", KeyID: "key", Secret: "secret",
			PublicOrigin: "http://127.0.0.1:5173", SessionTTL: time.Hour, RequestTimeout: time.Second,
		},
		&handlerWorkbenchRepo{binding: binding}, redisClient,
		&handlerWorkbenchKB{kb: &types.KnowledgeBase{ID: "kb-p", TenantID: 10000, Name: "Project KB"}},
		&handlerWorkbenchTenantRepo{tenant: &types.Tenant{ID: 10000}},
		knowledge, principalClient,
	)
	sessionToken, csrfToken, _, _, err := service.ExchangeTicket(context.Background(), "ticket", "PROJECT_P")
	require.NoError(t, err)
	return &workbenchHandlerFixture{
		handler: NewPixLabWorkbenchHandler(service), knowledge: knowledge,
		sessionToken: sessionToken, csrfToken: csrfToken, redis: redisClient,
		principalClient: principalClient,
	}
}

func TestPixLabWorkbenchStreamAuthorizationRevocationCancelsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	fixture.handler.streamAuthorizationCheckInterval = 5 * time.Millisecond
	fixture.principalClient.failValidation(errors.New("membership revoked"))

	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/pixlab-workbench/projects/PROJECT_P/sessions/s1/continue-stream", nil)
	c.Request.AddCookie(&http.Cookie{Name: appservice.PixLabWorkbenchCookieName, Value: fixture.sessionToken})
	c.Params = gin.Params{{Key: "project_code", Value: "PROJECT_P"}}

	stop := fixture.handler.watchStreamAuthorization(c)
	defer stop()

	select {
	case <-c.Request.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("stream request remained active after PixLab authorization was revoked")
	}
}

func TestPixLabWorkbenchResumeWithoutCookieIsQuiet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	router := gin.New()
	router.POST("/api/v1/pixlab-workbench/session/resume", fixture.handler.ResumeSession)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/pixlab-workbench/session/resume",
		bytes.NewBufferString(`{"project_code":"PROJECT_P"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"resumed":false`)
	require.NotContains(t, response.Body.String(), "csrf_token")
}

func TestPixLabWorkbenchResumeRotatesCSRFForValidCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	router := gin.New()
	router.POST("/api/v1/pixlab-workbench/session/resume", fixture.handler.ResumeSession)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/pixlab-workbench/session/resume",
		bytes.NewBufferString(`{"project_code":"PROJECT_P"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	request.AddCookie(&http.Cookie{Name: appservice.PixLabWorkbenchCookieName, Value: fixture.sessionToken})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"resumed":true`)
	require.Contains(t, response.Body.String(), `"project_code":"PROJECT_P"`)
	require.Contains(t, response.Body.String(), "csrf_token")
	require.NotContains(t, response.Body.String(), fixture.csrfToken)
	require.NotEmpty(t, response.Header().Values("Set-Cookie"))
}

func TestPixLabWorkbenchDocumentRetryCapabilityIsUploaderAndStatusScoped(t *testing.T) {
	principal := types.PixLabPrincipal{
		UserID:       "pixlab-user",
		Capabilities: []string{types.PixLabCapabilityRead, types.PixLabCapabilityUpload},
	}
	document := &types.Knowledge{
		ID:          "document-1",
		ParseStatus: types.ParseStatusFailed,
		Metadata:    types.JSON(`{"pixlab_uploader_user_id":"pixlab-user"}`),
	}

	require.True(t, newPixLabDocumentView(document, principal).CanReparse)

	otherUser := principal
	otherUser.UserID = "other-user"
	require.False(t, newPixLabDocumentView(document, otherUser).CanReparse)

	readOnly := principal
	readOnly.Capabilities = []string{types.PixLabCapabilityRead}
	require.False(t, newPixLabDocumentView(document, readOnly).CanReparse)

	completed := *document
	completed.ParseStatus = types.ParseStatusCompleted
	require.False(t, newPixLabDocumentView(&completed, principal).CanReparse)

	cancelled := *document
	cancelled.ParseStatus = types.ParseStatusCancelled
	require.True(t, newPixLabDocumentView(&cancelled, principal).CanReparse)
}

func TestPixLabWorkbenchUploadUsesBoundKnowledgeBaseAndTrustedMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	router := gin.New()
	router.POST(
		"/api/v1/pixlab-workbench/projects/:project_code/documents",
		fixture.handler.AuthenticateProject(), fixture.handler.UploadDocument,
	)

	request := multipartUploadRequest(t, "/api/v1/pixlab-workbench/projects/PROJECT_P/documents", "file", "notes.txt", "spec/notes.txt")
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	request.Header.Set("X-CSRF-Token", fixture.csrfToken)
	request.AddCookie(&http.Cookie{Name: appservice.PixLabWorkbenchCookieName, Value: fixture.sessionToken})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	require.Equal(t, 1, fixture.knowledge.createCalls)
	require.Equal(t, "kb-p", fixture.knowledge.kbID)
	require.Equal(t, "PROJECT_P", fixture.knowledge.metadata["pixlab_project_code"])
	require.Equal(t, "pixlab-user", fixture.knowledge.metadata["pixlab_uploader_user_id"])
	require.Equal(t, "pixlab:pixlab-user", fixture.knowledge.metadata["source_principal"])
	require.Equal(t, uint64(10000), fixture.knowledge.tenantID)
	require.Equal(t, uint64(10000), fixture.knowledge.tenantInfoID)
	require.Equal(t, "pixlab:pixlab-user", fixture.knowledge.userID)
}

func TestPixLabWorkbenchUploadRejectsTraversalBeforeKnowledgeService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	router := gin.New()
	router.POST(
		"/api/v1/pixlab-workbench/projects/:project_code/documents",
		fixture.handler.AuthenticateProject(), fixture.handler.UploadDocument,
	)

	request := multipartUploadRequest(t, "/api/v1/pixlab-workbench/projects/PROJECT_P/documents", "file", "notes.txt", "../secrets.txt")
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	request.Header.Set("X-CSRF-Token", fixture.csrfToken)
	request.AddCookie(&http.Cookie{Name: appservice.PixLabWorkbenchCookieName, Value: fixture.sessionToken})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "INVALID_FILE_NAME")
	require.Zero(t, fixture.knowledge.createCalls)
}

func TestPixLabWorkbenchReparseEnforcesUploaderStatusAndKnowledgeBase(t *testing.T) {
	tests := []struct {
		name       string
		document   *types.Knowledge
		wantStatus int
		wantCode   string
		wantCalls  int
	}{
		{
			name: "own failed document",
			document: &types.Knowledge{
				ID: "document-1", KnowledgeBaseID: "kb-p", ParseStatus: types.ParseStatusFailed,
				Metadata: types.JSON(`{"pixlab_uploader_user_id":"pixlab-user"}`),
			},
			wantStatus: http.StatusAccepted, wantCalls: 1,
		},
		{
			name: "another uploader",
			document: &types.Knowledge{
				ID: "document-1", KnowledgeBaseID: "kb-p", ParseStatus: types.ParseStatusFailed,
				Metadata: types.JSON(`{"pixlab_uploader_user_id":"other-user"}`),
			},
			wantStatus: http.StatusForbidden, wantCode: "PROJECT_FORBIDDEN",
		},
		{
			name: "completed document",
			document: &types.Knowledge{
				ID: "document-1", KnowledgeBaseID: "kb-p", ParseStatus: types.ParseStatusCompleted,
				Metadata: types.JSON(`{"pixlab_uploader_user_id":"pixlab-user"}`),
			},
			wantStatus: http.StatusConflict, wantCode: "DOCUMENT_NOT_RETRYABLE",
		},
		{
			name: "other knowledge base",
			document: &types.Knowledge{
				ID: "document-1", KnowledgeBaseID: "kb-q", ParseStatus: types.ParseStatusFailed,
				Metadata: types.JSON(`{"pixlab_uploader_user_id":"pixlab-user"}`),
			},
			wantStatus: http.StatusNotFound, wantCode: "DOCUMENT_NOT_FOUND",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			fixture := newWorkbenchHandlerFixture(t)
			fixture.knowledge.documents = map[string]*types.Knowledge{"document-1": test.document}
			router := gin.New()
			router.POST(
				"/api/v1/pixlab-workbench/projects/:project_code/documents/:document_id/reparse",
				fixture.handler.AuthenticateProject(), fixture.handler.ReparseDocument,
			)
			request := authenticatedWorkbenchRequest(
				t, fixture, http.MethodPost,
				"/api/v1/pixlab-workbench/projects/PROJECT_P/documents/document-1/reparse",
				bytes.NewBufferString(`{}`),
			)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			require.Equal(t, test.wantStatus, response.Code, response.Body.String())
			if test.wantCode != "" {
				require.Contains(t, response.Body.String(), test.wantCode)
			}
			require.Equal(t, test.wantCalls, fixture.knowledge.reparseCalls)
		})
	}
}

func TestPixLabWorkbenchWriteRequestsRequireCurrentCSRF(t *testing.T) {
	for _, csrf := range []string{"", "wrong-csrf"} {
		t.Run("csrf="+csrf, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			fixture := newWorkbenchHandlerFixture(t)
			fixture.knowledge.documents = map[string]*types.Knowledge{
				"document-1": {
					ID: "document-1", KnowledgeBaseID: "kb-p", ParseStatus: types.ParseStatusFailed,
					Metadata: types.JSON(`{"pixlab_uploader_user_id":"pixlab-user"}`),
				},
			}
			router := gin.New()
			router.POST(
				"/api/v1/pixlab-workbench/projects/:project_code/documents/:document_id/reparse",
				fixture.handler.AuthenticateProject(), fixture.handler.ReparseDocument,
			)
			request := authenticatedWorkbenchRequest(
				t, fixture, http.MethodPost,
				"/api/v1/pixlab-workbench/projects/PROJECT_P/documents/document-1/reparse",
				bytes.NewBufferString(`{}`),
			)
			request.Header.Set("X-CSRF-Token", csrf)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "CSRF_FAILED")
			require.Zero(t, fixture.knowledge.reparseCalls)
		})
	}
}

func TestPixLabWorkbenchSessionListIsScopedToUserAndProject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	projectP, projectQ := "PROJECT_P", "PROJECT_Q"
	sessions := &handlerSessionService{sessions: []*types.Session{
		{ID: "mine-p", TenantID: 10000, UserID: "pixlab:pixlab-user", PixLabProjectCode: &projectP, Title: "mine project p"},
		{ID: "mine-q", TenantID: 10000, UserID: "pixlab:pixlab-user", PixLabProjectCode: &projectQ, Title: "mine project q"},
		{ID: "other-p", TenantID: 10000, UserID: "pixlab:other-user", PixLabProjectCode: &projectP, Title: "other project p"},
	}}
	fixture.handler.sessions = sessions
	fixture.handler.messages = &handlerMessageService{}
	fixture.handler.sessionHandler = &sessionhandler.Handler{}
	router := gin.New()
	router.GET("/api/v1/pixlab-workbench/projects/:project_code/sessions", fixture.handler.AuthenticateProject(), fixture.handler.ListChatSessions)

	request := authenticatedWorkbenchRequest(t, fixture, http.MethodGet, "/api/v1/pixlab-workbench/projects/PROJECT_P/sessions", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "mine project p")
	require.NotContains(t, response.Body.String(), "mine project q")
	require.NotContains(t, response.Body.String(), "other project p")
}

func TestPixLabWorkbenchMessagesHideCrossUserAndCrossProjectSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	projectP, projectQ := "PROJECT_P", "PROJECT_Q"
	sessions := &handlerSessionService{sessions: []*types.Session{
		{ID: "other-user", TenantID: 10000, UserID: "pixlab:other-user", PixLabProjectCode: &projectP},
		{ID: "other-project", TenantID: 10000, UserID: "pixlab:pixlab-user", PixLabProjectCode: &projectQ},
	}}
	messages := &handlerMessageService{messages: map[string][]*types.Message{}}
	fixture.handler.sessions = sessions
	fixture.handler.messages = messages
	fixture.handler.sessionHandler = &sessionhandler.Handler{}
	router := gin.New()
	router.GET("/api/v1/pixlab-workbench/projects/:project_code/sessions/:session_id/messages", fixture.handler.AuthenticateProject(), fixture.handler.ChatMessages)

	for _, sessionID := range []string{"other-user", "other-project"} {
		request := authenticatedWorkbenchRequest(t, fixture, http.MethodGet, "/api/v1/pixlab-workbench/projects/PROJECT_P/sessions/"+sessionID+"/messages", nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), "SESSION_SCOPE_MISMATCH")
	}
	require.Zero(t, messages.calls, "message storage must not be queried for an out-of-scope session")
}

func TestPixLabWorkbenchDeleteAndStopHideCrossUserSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	projectP := "PROJECT_P"
	fixture.handler.sessions = &handlerSessionService{sessions: []*types.Session{
		{ID: "other-user-session", TenantID: 10000, UserID: "pixlab:other-user", PixLabProjectCode: &projectP},
	}}
	fixture.handler.messages = &handlerMessageService{}
	fixture.handler.sessionHandler = &sessionhandler.Handler{}
	router := gin.New()
	router.DELETE(
		"/api/v1/pixlab-workbench/projects/:project_code/sessions/:session_id",
		fixture.handler.AuthenticateProject(), fixture.handler.DeleteChatSession,
	)
	router.POST(
		"/api/v1/pixlab-workbench/projects/:project_code/sessions/:session_id/stop",
		fixture.handler.AuthenticateProject(), fixture.handler.StopChatAnswer,
	)

	for _, methodAndSuffix := range [][2]string{
		{http.MethodDelete, ""},
		{http.MethodPost, "/stop"},
	} {
		request := authenticatedWorkbenchRequest(
			t, fixture, methodAndSuffix[0],
			"/api/v1/pixlab-workbench/projects/PROJECT_P/sessions/other-user-session"+methodAndSuffix[1],
			bytes.NewBufferString(`{}`),
		)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), "SESSION_SCOPE_MISMATCH")
	}
}

func TestPixLabWorkbenchPreviewHidesOtherKnowledgeBase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	fixture.knowledge.documents = map[string]*types.Knowledge{
		"document-q": {ID: "document-q", KnowledgeBaseID: "kb-q"},
	}
	router := gin.New()
	router.GET(
		"/api/v1/pixlab-workbench/projects/:project_code/documents/:document_id/preview",
		fixture.handler.AuthenticateProject(), fixture.handler.PreviewDocument,
	)
	request := authenticatedWorkbenchRequest(
		t, fixture, http.MethodGet,
		"/api/v1/pixlab-workbench/projects/PROJECT_P/documents/document-q/preview",
		nil,
	)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "DOCUMENT_NOT_FOUND")
}

func TestPixLabWorkbenchMessageHistoryWhitelistsBoundKnowledgeBaseCitations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	projectP := "PROJECT_P"
	fixture.knowledge.documents = map[string]*types.Knowledge{
		"doc-p": {ID: "doc-p", KnowledgeBaseID: "kb-p", FileName: "allowed.md"},
		"doc-q": {ID: "doc-q", KnowledgeBaseID: "kb-q", FileName: "secret.md"},
	}
	fixture.handler.sessions = &handlerSessionService{sessions: []*types.Session{
		{ID: "mine-p", TenantID: 10000, UserID: "pixlab:pixlab-user", PixLabProjectCode: &projectP},
	}}
	fixture.handler.messages = &handlerMessageService{messages: map[string][]*types.Message{
		"mine-p": {{
			ID: "message-1", SessionID: "mine-p", Role: "assistant", Content: "answer", IsCompleted: true,
			KnowledgeReferences: types.References{
				{ID: "ref-p", KnowledgeID: "doc-p", KnowledgeBaseID: "kb-p", KnowledgeTitle: "Allowed"},
				{ID: "ref-q", KnowledgeID: "doc-q", KnowledgeBaseID: "kb-q", KnowledgeTitle: "Secret"},
			},
		}},
	}}
	fixture.handler.sessionHandler = &sessionhandler.Handler{}
	router := gin.New()
	router.GET("/api/v1/pixlab-workbench/projects/:project_code/sessions/:session_id/messages", fixture.handler.AuthenticateProject(), fixture.handler.ChatMessages)

	request := authenticatedWorkbenchRequest(t, fixture, http.MethodGet, "/api/v1/pixlab-workbench/projects/PROJECT_P/sessions/mine-p/messages", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "allowed.md")
	require.NotContains(t, response.Body.String(), "secret.md")
	require.NotContains(t, response.Body.String(), "ref-q")
}

func TestPixLabWorkbenchDocumentStagesHideDocumentsFromOtherKnowledgeBases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	fixture.knowledge.documents = map[string]*types.Knowledge{
		"doc-q": {ID: "doc-q", KnowledgeBaseID: "kb-q", FileName: "secret.md"},
	}
	router := gin.New()
	router.GET("/api/v1/pixlab-workbench/projects/:project_code/documents/:document_id/stages", fixture.handler.AuthenticateProject(), fixture.handler.DocumentStages)

	request := authenticatedWorkbenchRequest(t, fixture, http.MethodGet, "/api/v1/pixlab-workbench/projects/PROJECT_P/documents/doc-q/stages", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "DOCUMENT_NOT_FOUND")
	require.NotContains(t, response.Body.String(), "secret.md")
}

func TestPixLabWorkbenchProjectChunkReturnsSafeView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	chunks := &handlerChunkService{chunks: map[string]*types.Chunk{
		"chunk-p": {
			ID: "chunk-p", TenantID: 10000, KnowledgeID: "doc-p", KnowledgeBaseID: "kb-p",
			Content: "project content", ContentRevision: 4, ChunkIndex: 2,
			ChunkType: types.ChunkTypeText, ParentChunkID: "parent-p",
			LastEditorID: "internal-editor", Metadata: types.JSON(`{"private":"value"}`),
		},
	}}
	fixture.handler.chunks = chunks
	router := gin.New()
	router.GET("/api/v1/pixlab-workbench/projects/:project_code/chunks/:chunk_id", fixture.handler.AuthenticateProject(), fixture.handler.GetProjectChunk)

	request := authenticatedWorkbenchRequest(t, fixture, http.MethodGet, "/api/v1/pixlab-workbench/projects/PROJECT_P/chunks/chunk-p", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"id":"chunk-p"`)
	require.Contains(t, response.Body.String(), `"knowledge_base_id":"kb-p"`)
	require.Contains(t, response.Body.String(), `"content":"project content"`)
	require.NotContains(t, response.Body.String(), "tenant_id")
	require.NotContains(t, response.Body.String(), "last_editor_id")
	require.NotContains(t, response.Body.String(), "metadata")
	require.NotContains(t, response.Body.String(), "internal-editor")
	require.NotContains(t, response.Body.String(), "private")
	require.Equal(t, 1, chunks.getCalls)
}

func TestPixLabWorkbenchProjectChunkHidesOtherKnowledgeBase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	chunks := &handlerChunkService{chunks: map[string]*types.Chunk{
		"chunk-q": {ID: "chunk-q", TenantID: 10000, KnowledgeID: "doc-q", KnowledgeBaseID: "kb-q", Content: "secret"},
	}}
	fixture.handler.chunks = chunks
	router := gin.New()
	router.GET("/api/v1/pixlab-workbench/projects/:project_code/chunks/:chunk_id", fixture.handler.AuthenticateProject(), fixture.handler.GetProjectChunk)

	request := authenticatedWorkbenchRequest(t, fixture, http.MethodGet, "/api/v1/pixlab-workbench/projects/PROJECT_P/chunks/chunk-q", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "CHUNK_NOT_FOUND")
	require.NotContains(t, response.Body.String(), "secret")
	require.NotContains(t, response.Body.String(), "kb-q")
}

func TestPixLabWorkbenchDocumentChunkRequiresMatchingDocument(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	chunks := &handlerChunkService{chunks: map[string]*types.Chunk{
		"chunk-p": {ID: "chunk-p", KnowledgeID: "doc-p", KnowledgeBaseID: "kb-p", Content: "project content"},
	}}
	fixture.handler.chunks = chunks
	router := gin.New()
	router.GET("/api/v1/pixlab-workbench/projects/:project_code/documents/:document_id/chunks/:chunk_id", fixture.handler.AuthenticateProject(), fixture.handler.GetDocumentChunk)

	request := authenticatedWorkbenchRequest(t, fixture, http.MethodGet, "/api/v1/pixlab-workbench/projects/PROJECT_P/documents/doc-other/chunks/chunk-p", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "CHUNK_NOT_FOUND")
	require.NotContains(t, response.Body.String(), "project content")
}

func TestPixLabWorkbenchDocumentChunkListRejectsOtherKnowledgeBaseBeforeChunkQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newWorkbenchHandlerFixture(t)
	fixture.knowledge.documents = map[string]*types.Knowledge{
		"doc-q": {ID: "doc-q", KnowledgeBaseID: "kb-q", FileName: "secret.md"},
	}
	chunks := &handlerChunkService{list: []*types.Chunk{{ID: "chunk-q", KnowledgeID: "doc-q", KnowledgeBaseID: "kb-q"}}}
	fixture.handler.chunks = chunks
	router := gin.New()
	router.GET("/api/v1/pixlab-workbench/projects/:project_code/documents/:document_id/chunks", fixture.handler.AuthenticateProject(), fixture.handler.ListDocumentChunks)

	request := authenticatedWorkbenchRequest(t, fixture, http.MethodGet, "/api/v1/pixlab-workbench/projects/PROJECT_P/documents/doc-q/chunks", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "DOCUMENT_NOT_FOUND")
	require.Zero(t, chunks.listCalls, "chunk storage must not be queried for an out-of-scope document")
	require.NotContains(t, response.Body.String(), "secret.md")
}

func TestPixLabWorkbenchDocumentListRejectsInvalidSortValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, query := range []string{"sort_by=tenant_id", "sort_order=sideways"} {
		t.Run(query, func(t *testing.T) {
			fixture := newWorkbenchHandlerFixture(t)
			router := gin.New()
			router.GET("/api/v1/pixlab-workbench/projects/:project_code/documents", fixture.handler.AuthenticateProject(), fixture.handler.ListDocuments)

			request := authenticatedWorkbenchRequest(t, fixture, http.MethodGet, "/api/v1/pixlab-workbench/projects/PROJECT_P/documents?"+query, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "INVALID_REQUEST")
		})
	}
}

func authenticatedWorkbenchRequest(
	t *testing.T, fixture *workbenchHandlerFixture, method, target string, body io.Reader,
) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, target, body)
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	if method != http.MethodGet && method != http.MethodHead {
		request.Header.Set("X-CSRF-Token", fixture.csrfToken)
	}
	request.AddCookie(&http.Cookie{Name: appservice.PixLabWorkbenchCookieName, Value: fixture.sessionToken})
	return request
}

func multipartUploadRequest(
	t *testing.T,
	target, fieldName, uploadName, customFileName string,
) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="`+fieldName+`"; filename="`+uploadName+`"`)
	header.Set("Content-Type", "text/plain")
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = io.WriteString(part, "unique PixLab embedding test content")
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("fileName", customFileName))
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}
