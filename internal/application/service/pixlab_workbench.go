package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/redis/go-redis/v9"
)

const (
	PixLabWorkbenchCookieName = "wk_pixlab"
	workbenchSessionPrefix    = "pixlab:workbench:session:"
	maxBackchannelBodyBytes   = 1 << 20
)

// PixLabWorkbenchError is safe to expose through the dedicated workbench API.
type PixLabWorkbenchError struct {
	Status  int
	Code    string
	Message string
	Cause   error
}

func (e *PixLabWorkbenchError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *PixLabWorkbenchError) Unwrap() error { return e.Cause }

func workbenchError(status int, code, message string, cause error) error {
	return &PixLabWorkbenchError{Status: status, Code: code, Message: message, Cause: cause}
}

type PixLabWorkbenchConfig struct {
	Enabled         bool
	InternalBaseURL string
	KeyID           string
	Secret          string
	PublicOrigin    string
	SessionTTL      time.Duration
	RequestTimeout  time.Duration
}

func LoadPixLabWorkbenchConfig() PixLabWorkbenchConfig {
	return PixLabWorkbenchConfig{
		Enabled:         parseEnvBool("PIXLAB_BRIDGE_ENABLED", false),
		InternalBaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("PIXLAB_INTERNAL_BASE_URL")), "/"),
		KeyID:           strings.TrimSpace(os.Getenv("PIXLAB_BACKCHANNEL_KEY_ID")),
		Secret:          os.Getenv("PIXLAB_BACKCHANNEL_SECRET"),
		PublicOrigin:    strings.TrimRight(strings.TrimSpace(os.Getenv("PIXLAB_PUBLIC_ORIGIN")), "/"),
		SessionTTL:      parseEnvDuration("PIXLAB_WORKBENCH_SESSION_TTL", time.Hour),
		RequestTimeout:  parseEnvDuration("PIXLAB_BACKCHANNEL_TIMEOUT", 10*time.Second),
	}
}

func parseEnvBool(name string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	return err == nil && value
}

func parseEnvDuration(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if duration, err := time.ParseDuration(raw); err == nil && duration > 0 {
		return duration
	}
	return fallback
}

func (c PixLabWorkbenchConfig) Validate() error {
	if !c.Enabled {
		return workbenchError(http.StatusNotFound, "WORKBENCH_DISABLED", "Project knowledge workbench is disabled", nil)
	}
	if c.InternalBaseURL == "" || c.KeyID == "" || c.Secret == "" || c.PublicOrigin == "" {
		return workbenchError(http.StatusServiceUnavailable, "BACKCHANNEL_NOT_CONFIGURED", "PixLab bridge is not configured", nil)
	}
	internalURL, err := url.Parse(c.InternalBaseURL)
	if err != nil || internalURL.Scheme == "" || internalURL.Host == "" {
		return workbenchError(http.StatusServiceUnavailable, "BACKCHANNEL_NOT_CONFIGURED", "PixLab internal URL is invalid", err)
	}
	publicURL, err := url.Parse(c.PublicOrigin)
	if err != nil || publicURL.Scheme == "" || publicURL.Host == "" || publicURL.Path != "" {
		return workbenchError(http.StatusServiceUnavailable, "BACKCHANNEL_NOT_CONFIGURED", "PixLab public origin is invalid", err)
	}
	if c.SessionTTL < time.Minute {
		return workbenchError(http.StatusServiceUnavailable, "BACKCHANNEL_NOT_CONFIGURED", "Workbench session TTL must be at least one minute", nil)
	}
	return nil
}

func (c PixLabWorkbenchConfig) CookieSecure() bool {
	parsed, err := url.Parse(c.PublicOrigin)
	return err == nil && strings.EqualFold(parsed.Scheme, "https")
}

type PixLabPrincipalClient interface {
	Redeem(ctx context.Context, ticket, projectCode string) (*types.PixLabPrincipal, error)
	Validate(ctx context.Context, principal types.PixLabPrincipal) (*types.PixLabPrincipal, error)
}

type pixLabHTTPClient struct {
	config PixLabWorkbenchConfig
	client *http.Client
	now    func() time.Time
}

func newPixLabHTTPClient(config PixLabWorkbenchConfig) PixLabPrincipalClient {
	return &pixLabHTTPClient{
		config: config,
		client: &http.Client{Timeout: config.RequestTimeout},
		now:    time.Now,
	}
}

func (c *pixLabHTTPClient) Redeem(ctx context.Context, ticket, projectCode string) (*types.PixLabPrincipal, error) {
	return c.post(ctx, "/api/v1/internal/weknora/tickets/redeem", map[string]string{
		"ticket": ticket, "project_code": projectCode,
	})
}

func (c *pixLabHTTPClient) Validate(ctx context.Context, principal types.PixLabPrincipal) (*types.PixLabPrincipal, error) {
	payload := struct {
		UserID            string `json:"user_id"`
		ProjectCode       string `json:"project_code"`
		PixLabSessionID   string `json:"pixlab_session_id"`
		PermissionVersion int    `json:"permission_version"`
		BindingRevision   int    `json:"binding_revision"`
	}{
		UserID: principal.UserID, ProjectCode: principal.ProjectCode,
		PixLabSessionID:   principal.PixLabSessionID,
		PermissionVersion: principal.PermissionVersion,
		BindingRevision:   principal.BindingRevision,
	}
	return c.post(ctx, "/api/v1/internal/weknora/principals/validate", payload)
}

func (c *pixLabHTTPClient) post(ctx context.Context, path string, payload any) (*types.PixLabPrincipal, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, workbenchError(http.StatusInternalServerError, "WEKNORA_ERROR", "Failed to encode PixLab request", err)
	}
	nonce, err := randomToken(18)
	if err != nil {
		return nil, workbenchError(http.StatusInternalServerError, "WEKNORA_ERROR", "Failed to create request nonce", err)
	}
	timestamp := strconv.FormatInt(c.now().Unix(), 10)
	bodyHash := sha256.Sum256(body)
	canonical := strings.Join([]string{
		timestamp, nonce, http.MethodPost, path, hex.EncodeToString(bodyHash[:]),
	}, "\n")
	signer := hmac.New(sha256.New, []byte(c.config.Secret))
	_, _ = signer.Write([]byte(canonical))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.InternalBaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, workbenchError(http.StatusBadGateway, "PIXLAB_UNAVAILABLE", "Failed to create PixLab request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PixLab-Key-Id", c.config.KeyID)
	req.Header.Set("X-PixLab-Timestamp", timestamp)
	req.Header.Set("X-PixLab-Nonce", nonce)
	req.Header.Set("X-PixLab-Signature", hex.EncodeToString(signer.Sum(nil)))

	response, err := c.client.Do(req)
	if err != nil {
		return nil, workbenchError(http.StatusBadGateway, "PIXLAB_UNAVAILABLE", "PixLab authorization service is unavailable", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxBackchannelBodyBytes))
	if err != nil {
		return nil, workbenchError(http.StatusBadGateway, "PIXLAB_UNAVAILABLE", "Failed to read PixLab response", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, decodePixLabError(response.StatusCode, responseBody)
	}
	var envelope struct {
		Data types.PixLabPrincipal `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return nil, workbenchError(http.StatusBadGateway, "PIXLAB_UNAVAILABLE", "PixLab returned an invalid response", err)
	}
	if envelope.Data.UserID == "" || envelope.Data.ProjectCode == "" || envelope.Data.PixLabSessionID == "" {
		return nil, workbenchError(http.StatusBadGateway, "PIXLAB_UNAVAILABLE", "PixLab returned an incomplete principal", nil)
	}
	return &envelope.Data, nil
}

func decodePixLabError(status int, body []byte) error {
	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Detail  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"detail"`
	}
	_ = json.Unmarshal(body, &payload)
	if payload.Code == "" {
		payload.Code, payload.Message = payload.Detail.Code, payload.Detail.Message
	}
	if payload.Code == "" {
		payload.Code = "PIXLAB_AUTHORIZATION_FAILED"
	}
	if payload.Message == "" {
		payload.Message = "PixLab rejected the workbench authorization"
	}
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	return workbenchError(status, payload.Code, payload.Message, nil)
}

type PixLabWorkbenchService struct {
	config     PixLabWorkbenchConfig
	repository interfaces.PixLabWorkbenchRepository
	redis      *redis.Client
	kbService  interfaces.KnowledgeBaseService
	tenantRepo interfaces.TenantRepository
	knowledge  interfaces.KnowledgeService
	principal  PixLabPrincipalClient
}

func NewPixLabWorkbenchService(
	repository interfaces.PixLabWorkbenchRepository,
	redisClient *redis.Client,
	kbService interfaces.KnowledgeBaseService,
	tenantRepo interfaces.TenantRepository,
	knowledge interfaces.KnowledgeService,
) *PixLabWorkbenchService {
	config := LoadPixLabWorkbenchConfig()
	return NewPixLabWorkbenchServiceWithConfig(config, repository, redisClient, kbService, tenantRepo, knowledge, nil)
}

func NewPixLabWorkbenchServiceWithConfig(
	config PixLabWorkbenchConfig,
	repository interfaces.PixLabWorkbenchRepository,
	redisClient *redis.Client,
	kbService interfaces.KnowledgeBaseService,
	tenantRepo interfaces.TenantRepository,
	knowledge interfaces.KnowledgeService,
	principalClient PixLabPrincipalClient,
) *PixLabWorkbenchService {
	if principalClient == nil {
		principalClient = newPixLabHTTPClient(config)
	}
	return &PixLabWorkbenchService{
		config: config, repository: repository, redis: redisClient,
		kbService: kbService, tenantRepo: tenantRepo, knowledge: knowledge, principal: principalClient,
	}
}

func (s *PixLabWorkbenchService) Config() PixLabWorkbenchConfig { return s.config }

func (s *PixLabWorkbenchService) RequireEnabled() error {
	if err := s.config.Validate(); err != nil {
		return err
	}
	if s.redis == nil {
		return workbenchError(http.StatusServiceUnavailable, "SESSION_STORE_UNAVAILABLE", "Workbench session storage is unavailable", nil)
	}
	return nil
}

func (s *PixLabWorkbenchService) ExchangeTicket(
	ctx context.Context,
	ticket, projectCode string,
) (string, string, *types.PixLabPrincipal, *types.PixLabProjectBinding, error) {
	if err := s.RequireEnabled(); err != nil {
		return "", "", nil, nil, err
	}
	projectCode, err := normalizeProjectCode(projectCode)
	if err != nil {
		return "", "", nil, nil, err
	}
	if strings.TrimSpace(ticket) == "" {
		return "", "", nil, nil, workbenchError(http.StatusBadRequest, "TICKET_REQUIRED", "Workbench ticket is required", nil)
	}
	principal, err := s.principal.Redeem(ctx, ticket, projectCode)
	if err != nil {
		return "", "", nil, nil, err
	}
	if principal.ProjectCode != projectCode {
		return "", "", nil, nil, workbenchError(http.StatusForbidden, "PROJECT_FORBIDDEN", "Ticket does not belong to this project", nil)
	}
	if !principal.HasCapability(types.PixLabCapabilityRead) {
		return "", "", nil, nil, workbenchError(http.StatusForbidden, "PROJECT_FORBIDDEN", "Principal cannot access this project", nil)
	}
	binding, err := s.resolveBinding(ctx, *principal)
	if err != nil {
		return "", "", nil, nil, err
	}
	sessionToken, err := randomToken(32)
	if err != nil {
		return "", "", nil, nil, workbenchError(http.StatusInternalServerError, "WEKNORA_ERROR", "Failed to create workbench session", err)
	}
	csrfToken, err := randomToken(32)
	if err != nil {
		return "", "", nil, nil, workbenchError(http.StatusInternalServerError, "WEKNORA_ERROR", "Failed to create CSRF token", err)
	}
	session := types.PixLabWorkbenchSession{
		Principal: *principal,
		CSRFHash:  sha256Hex(csrfToken),
		CreatedAt: time.Now().UTC(),
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return "", "", nil, nil, workbenchError(http.StatusInternalServerError, "WEKNORA_ERROR", "Failed to encode workbench session", err)
	}
	if err := s.redis.Set(ctx, sessionRedisKey(sessionToken), raw, s.config.SessionTTL).Err(); err != nil {
		return "", "", nil, nil, workbenchError(http.StatusServiceUnavailable, "SESSION_STORE_UNAVAILABLE", "Failed to persist workbench session", err)
	}
	return sessionToken, csrfToken, principal, binding, nil
}

func (s *PixLabWorkbenchService) Authenticate(
	ctx context.Context,
	sessionToken, projectCode, csrfToken string,
	requireCSRF bool,
) (*types.PixLabWorkbenchSession, *types.PixLabProjectBinding, context.Context, error) {
	if err := s.RequireEnabled(); err != nil {
		return nil, nil, ctx, err
	}
	projectCode, err := normalizeProjectCode(projectCode)
	if err != nil {
		return nil, nil, ctx, err
	}
	if strings.TrimSpace(sessionToken) == "" {
		return nil, nil, ctx, workbenchError(http.StatusUnauthorized, "UNAUTHENTICATED", "Workbench session is missing", nil)
	}
	raw, err := s.redis.Get(ctx, sessionRedisKey(sessionToken)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil, ctx, workbenchError(http.StatusUnauthorized, "UNAUTHENTICATED", "Workbench session expired", nil)
	}
	if err != nil {
		return nil, nil, ctx, workbenchError(http.StatusServiceUnavailable, "SESSION_STORE_UNAVAILABLE", "Failed to read workbench session", err)
	}
	var session types.PixLabWorkbenchSession
	if err := json.Unmarshal(raw, &session); err != nil {
		return nil, nil, ctx, workbenchError(http.StatusUnauthorized, "UNAUTHENTICATED", "Workbench session is invalid", err)
	}
	if session.Principal.ProjectCode != projectCode {
		return nil, nil, ctx, workbenchError(http.StatusNotFound, "SESSION_SCOPE_MISMATCH", "Workbench resource was not found", nil)
	}
	if requireCSRF && !hmac.Equal([]byte(session.CSRFHash), []byte(sha256Hex(csrfToken))) {
		return nil, nil, ctx, workbenchError(http.StatusForbidden, "CSRF_FAILED", "CSRF validation failed", nil)
	}
	validated, err := s.principal.Validate(ctx, session.Principal)
	if err != nil {
		return nil, nil, ctx, err
	}
	if validated.UserID != session.Principal.UserID || validated.ProjectCode != projectCode ||
		validated.PixLabSessionID != session.Principal.PixLabSessionID ||
		validated.PermissionVersion != session.Principal.PermissionVersion ||
		validated.BindingRevision != session.Principal.BindingRevision {
		return nil, nil, ctx, workbenchError(http.StatusUnauthorized, "UNAUTHENTICATED", "PixLab principal changed", nil)
	}
	if !validated.HasCapability(types.PixLabCapabilityRead) {
		return nil, nil, ctx, workbenchError(http.StatusForbidden, "PROJECT_FORBIDDEN", "Principal cannot access this project", nil)
	}
	session.Principal.Capabilities = validated.Capabilities
	binding, err := s.resolveBinding(ctx, session.Principal)
	if err != nil {
		return nil, nil, ctx, err
	}
	if s.tenantRepo == nil {
		return nil, nil, ctx, workbenchError(http.StatusServiceUnavailable, "WEKNORA_ERROR", "Tenant repository is unavailable", nil)
	}
	tenant, err := s.tenantRepo.GetTenantByID(ctx, binding.TenantID)
	if err != nil || tenant == nil || tenant.ID != binding.TenantID {
		return nil, nil, ctx, workbenchError(http.StatusConflict, "BINDING_NOT_READY", "Bound tenant is unavailable", err)
	}
	workbenchUserID := "pixlab:" + session.Principal.UserID
	workbenchRole := types.TenantRoleViewer
	if session.Principal.HasCapability(types.PixLabCapabilityUpload) {
		workbenchRole = types.TenantRoleContributor
	}
	scoped := types.WithCaller(ctx, types.Caller{
		TenantID: binding.TenantID,
		UserID:   workbenchUserID,
		Role:     workbenchRole,
	})
	scoped = types.WithExecutionTenant(scoped, binding.TenantID)
	scoped = context.WithValue(scoped, types.TenantInfoContextKey, tenant)
	scoped = context.WithValue(scoped, types.UserIDContextKey, workbenchUserID)
	scoped = context.WithValue(scoped, types.TenantRoleContextKey, workbenchRole)
	return &session, binding, scoped, nil
}

// ResumeSession revalidates an existing HttpOnly workbench session and rotates
// the in-memory CSRF credential without extending the Redis session lifetime.
func (s *PixLabWorkbenchService) ResumeSession(
	ctx context.Context,
	sessionToken, projectCode string,
) (string, time.Duration, *types.PixLabWorkbenchSession, *types.PixLabProjectBinding, error) {
	session, binding, _, err := s.Authenticate(ctx, sessionToken, projectCode, "", false)
	if err != nil {
		return "", 0, nil, nil, err
	}

	remaining, err := s.redis.PTTL(ctx, sessionRedisKey(sessionToken)).Result()
	if err != nil || remaining <= 0 {
		return "", 0, nil, nil, workbenchError(
			http.StatusUnauthorized,
			"UNAUTHENTICATED",
			"Workbench session expired",
			err,
		)
	}
	csrfToken, err := randomToken(32)
	if err != nil {
		return "", 0, nil, nil, workbenchError(
			http.StatusInternalServerError,
			"WEKNORA_ERROR",
			"Failed to create CSRF token",
			err,
		)
	}
	session.CSRFHash = sha256Hex(csrfToken)
	raw, err := json.Marshal(session)
	if err != nil {
		return "", 0, nil, nil, workbenchError(
			http.StatusInternalServerError,
			"WEKNORA_ERROR",
			"Failed to encode workbench session",
			err,
		)
	}
	result, err := s.redis.SetArgs(ctx, sessionRedisKey(sessionToken), raw, redis.SetArgs{
		Mode: "XX",
		TTL:  remaining,
	}).Result()
	if errors.Is(err, redis.Nil) || (err == nil && result == "") {
		return "", 0, nil, nil, workbenchError(
			http.StatusUnauthorized,
			"UNAUTHENTICATED",
			"Workbench session expired",
			err,
		)
	}
	if err != nil {
		return "", 0, nil, nil, workbenchError(
			http.StatusServiceUnavailable,
			"SESSION_STORE_UNAVAILABLE",
			"Failed to rotate workbench session",
			err,
		)
	}
	return csrfToken, remaining, session, binding, nil
}

func (s *PixLabWorkbenchService) DeleteSession(ctx context.Context, sessionToken string) error {
	if s.redis == nil || strings.TrimSpace(sessionToken) == "" {
		return nil
	}
	if err := s.redis.Del(ctx, sessionRedisKey(sessionToken)).Err(); err != nil {
		return workbenchError(http.StatusServiceUnavailable, "SESSION_STORE_UNAVAILABLE", "Failed to delete workbench session", err)
	}
	return nil
}

func (s *PixLabWorkbenchService) DeleteAuthenticatedSession(
	ctx context.Context,
	sessionToken, csrfToken string,
) error {
	if err := s.RequireEnabled(); err != nil {
		return err
	}
	if strings.TrimSpace(sessionToken) == "" {
		return workbenchError(http.StatusUnauthorized, "UNAUTHENTICATED", "Workbench session is missing", nil)
	}
	raw, err := s.redis.Get(ctx, sessionRedisKey(sessionToken)).Bytes()
	if errors.Is(err, redis.Nil) {
		return workbenchError(http.StatusUnauthorized, "UNAUTHENTICATED", "Workbench session expired", nil)
	}
	if err != nil {
		return workbenchError(http.StatusServiceUnavailable, "SESSION_STORE_UNAVAILABLE", "Failed to read workbench session", err)
	}
	var session types.PixLabWorkbenchSession
	if err := json.Unmarshal(raw, &session); err != nil {
		return workbenchError(http.StatusUnauthorized, "UNAUTHENTICATED", "Workbench session is invalid", err)
	}
	if !hmac.Equal([]byte(session.CSRFHash), []byte(sha256Hex(csrfToken))) {
		return workbenchError(http.StatusForbidden, "CSRF_FAILED", "CSRF validation failed", nil)
	}
	return s.DeleteSession(ctx, sessionToken)
}

func (s *PixLabWorkbenchService) resolveBinding(
	ctx context.Context,
	principal types.PixLabPrincipal,
) (*types.PixLabProjectBinding, error) {
	binding, err := s.repository.GetProjectBinding(ctx, principal.ProjectCode)
	if errors.Is(err, repository.ErrPixLabProjectBindingNotFound) {
		return nil, workbenchError(http.StatusConflict, "BINDING_NOT_READY", "Project knowledge workbench is not configured", nil)
	}
	if err != nil {
		return nil, workbenchError(http.StatusInternalServerError, "WEKNORA_ERROR", "Failed to load project binding", err)
	}
	if binding.Status != types.PixLabBindingStatusActive || binding.Revision != principal.BindingRevision {
		return nil, workbenchError(http.StatusConflict, "BINDING_CHANGED", "Project knowledge binding changed", nil)
	}
	if binding.TenantID == 0 || binding.KnowledgeBaseID == "" || binding.AgentID == "" {
		return nil, workbenchError(http.StatusConflict, "BINDING_NOT_READY", "Project knowledge binding is incomplete", nil)
	}
	ctx = types.WithExecutionTenant(ctx, binding.TenantID)
	kb, err := s.kbService.GetKnowledgeBaseByID(ctx, binding.KnowledgeBaseID)
	if err != nil || kb == nil || kb.TenantID != binding.TenantID {
		return nil, workbenchError(http.StatusConflict, "BINDING_NOT_READY", "Bound knowledge base is unavailable", err)
	}
	return binding, nil
}

func (s *PixLabWorkbenchService) KnowledgeService() interfaces.KnowledgeService {
	return s.knowledge
}

func (s *PixLabWorkbenchService) KnowledgeBaseService() interfaces.KnowledgeBaseService {
	return s.kbService
}

func normalizeProjectCode(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return "", workbenchError(http.StatusBadRequest, "PROJECT_CODE_INVALID", "Project code is invalid", nil)
	}
	for _, current := range value {
		if (current >= 'a' && current <= 'z') || (current >= 'A' && current <= 'Z') ||
			(current >= '0' && current <= '9') || current == '_' || current == '-' || current == '.' {
			continue
		}
		return "", workbenchError(http.StatusBadRequest, "PROJECT_CODE_INVALID", "Project code is invalid", nil)
	}
	return value, nil
}

func randomToken(byteCount int) (string, error) {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func sha256Hex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func sessionRedisKey(token string) string {
	return workbenchSessionPrefix + sha256Hex(token)
}
