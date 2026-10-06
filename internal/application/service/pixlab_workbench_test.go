package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type pixLabWorkbenchRepoStub struct {
	binding *types.PixLabProjectBinding
	err     error
}

func (r *pixLabWorkbenchRepoStub) GetProjectBinding(
	_ context.Context,
	projectCode string,
) (*types.PixLabProjectBinding, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.binding == nil || r.binding.ProjectCode != projectCode {
		return nil, errTestBindingNotFound
	}
	copy := *r.binding
	return &copy, nil
}

var errTestBindingNotFound = &PixLabWorkbenchError{
	Status: http.StatusNotFound, Code: "TEST_NOT_FOUND", Message: "not found",
}

type pixLabKBServiceStub struct {
	interfaces.KnowledgeBaseService
	kb *types.KnowledgeBase
}

func (s *pixLabKBServiceStub) GetKnowledgeBaseByID(_ context.Context, id string) (*types.KnowledgeBase, error) {
	if s.kb == nil || s.kb.ID != id {
		return nil, errTestBindingNotFound
	}
	copy := *s.kb
	return &copy, nil
}

type pixLabKnowledgeServiceStub struct {
	interfaces.KnowledgeService
}

type pixLabTenantRepoStub struct {
	interfaces.TenantRepository
	tenant *types.Tenant
}

func (s *pixLabTenantRepoStub) GetTenantByID(_ context.Context, id uint64) (*types.Tenant, error) {
	if s.tenant == nil || s.tenant.ID != id {
		return nil, errTestBindingNotFound
	}
	copy := *s.tenant
	return &copy, nil
}

type pixLabPrincipalClientStub struct {
	principal     types.PixLabPrincipal
	redeemCalls   int
	validateCalls int
}

func (s *pixLabPrincipalClientStub) Redeem(_ context.Context, _, _ string) (*types.PixLabPrincipal, error) {
	s.redeemCalls++
	copy := s.principal
	return &copy, nil
}

func (s *pixLabPrincipalClientStub) Validate(_ context.Context, _ types.PixLabPrincipal) (*types.PixLabPrincipal, error) {
	s.validateCalls++
	copy := s.principal
	return &copy, nil
}

func TestPixLabBackchannelSignsCanonicalPayload(t *testing.T) {
	t.Parallel()
	fixedNow := time.Unix(1_800_000_000, 0)
	const secret = "test-backchannel-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		timestamp := request.Header.Get("X-PixLab-Timestamp")
		nonce := request.Header.Get("X-PixLab-Nonce")
		digest := sha256.Sum256(body)
		canonical := strings.Join([]string{
			timestamp, nonce, http.MethodPost, request.URL.Path, hex.EncodeToString(digest[:]),
		}, "\n")
		signer := hmac.New(sha256.New, []byte(secret))
		_, _ = signer.Write([]byte(canonical))
		require.Equal(t, "test-key", request.Header.Get("X-PixLab-Key-Id"))
		require.Equal(t, hex.EncodeToString(signer.Sum(nil)), request.Header.Get("X-PixLab-Signature"))
		require.Equal(t, "1800000000", timestamp)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"user_id":"u1","project_code":"PROJECT_P","pixlab_session_id":"s1","permission_version":2,"binding_revision":3,"capabilities":["read","upload","chat"]}}`)
	}))
	defer server.Close()

	config := PixLabWorkbenchConfig{
		Enabled: true, InternalBaseURL: server.URL, KeyID: "test-key", Secret: secret,
		PublicOrigin: "http://127.0.0.1:5173", SessionTTL: time.Hour, RequestTimeout: time.Second,
	}
	client := &pixLabHTTPClient{config: config, client: server.Client(), now: func() time.Time { return fixedNow }}
	principal, err := client.Redeem(context.Background(), "one-time-ticket", "PROJECT_P")
	require.NoError(t, err)
	require.Equal(t, "u1", principal.UserID)
	require.Equal(t, 3, principal.BindingRevision)
}

func TestPixLabWorkbenchSessionScopesProjectCSRFAndBinding(t *testing.T) {
	t.Parallel()
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	binding := &types.PixLabProjectBinding{
		ProjectCode: "PROJECT_P", ProjectName: "Project P", TenantID: 10000,
		KnowledgeBaseID: "kb-p", AgentID: "agent-p", Status: types.PixLabBindingStatusActive, Revision: 3,
	}
	principalClient := &pixLabPrincipalClientStub{principal: types.PixLabPrincipal{
		UserID: "pixlab-user", ProjectCode: "PROJECT_P", PixLabSessionID: "pixlab-session",
		PermissionVersion: 2, BindingRevision: 3,
		Capabilities: []string{types.PixLabCapabilityRead, types.PixLabCapabilityUpload, types.PixLabCapabilityChat},
	}}
	service := NewPixLabWorkbenchServiceWithConfig(
		PixLabWorkbenchConfig{
			Enabled: true, InternalBaseURL: "http://pixlab.test", KeyID: "key", Secret: "secret",
			PublicOrigin: "http://127.0.0.1:5173", SessionTTL: time.Hour, RequestTimeout: time.Second,
		},
		&pixLabWorkbenchRepoStub{binding: binding}, redisClient,
		&pixLabKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-p", TenantID: 10000}},
		&pixLabTenantRepoStub{tenant: &types.Tenant{ID: 10000}},
		&pixLabKnowledgeServiceStub{}, principalClient,
	)

	sessionToken, csrfToken, _, _, err := service.ExchangeTicket(context.Background(), "ticket", "PROJECT_P")
	require.NoError(t, err)
	require.NotEmpty(t, sessionToken)
	require.NotEmpty(t, csrfToken)
	require.NotContains(t, mini.Keys()[0], sessionToken, "raw session token must not be used as the Redis key")

	session, resolved, scoped, err := service.Authenticate(
		context.Background(), sessionToken, "PROJECT_P", csrfToken, true,
	)
	require.NoError(t, err)
	require.Equal(t, "pixlab-user", session.Principal.UserID)
	require.Equal(t, "kb-p", resolved.KnowledgeBaseID)
	tenantID, ok := types.TenantIDFromContext(scoped)
	require.True(t, ok)
	require.Equal(t, uint64(10000), tenantID)
	tenant, ok := scoped.Value(types.TenantInfoContextKey).(*types.Tenant)
	require.True(t, ok)
	require.Equal(t, uint64(10000), tenant.ID)
	userID, ok := types.UserIDFromContext(scoped)
	require.True(t, ok)
	require.Equal(t, "pixlab:pixlab-user", userID)
	require.Equal(t, types.TenantRoleContributor, types.TenantRoleFromContext(scoped))
	require.Equal(t, types.Caller{
		TenantID: 10000,
		UserID:   "pixlab:pixlab-user",
		Role:     types.TenantRoleContributor,
	}, types.CallerFromContext(scoped))

	_, _, _, err = service.Authenticate(context.Background(), sessionToken, "PROJECT_OTHER", csrfToken, true)
	assertWorkbenchError(t, err, http.StatusNotFound, "SESSION_SCOPE_MISMATCH")
	_, _, _, err = service.Authenticate(context.Background(), sessionToken, "PROJECT_P", "wrong", true)
	assertWorkbenchError(t, err, http.StatusForbidden, "CSRF_FAILED")

	binding.Revision = 4
	_, _, _, err = service.Authenticate(context.Background(), sessionToken, "PROJECT_P", csrfToken, true)
	assertWorkbenchError(t, err, http.StatusConflict, "BINDING_CHANGED")

	err = service.DeleteAuthenticatedSession(context.Background(), sessionToken, "wrong")
	assertWorkbenchError(t, err, http.StatusForbidden, "CSRF_FAILED")
	binding.Revision = 3
	require.NoError(t, service.DeleteAuthenticatedSession(context.Background(), sessionToken, csrfToken))
	require.Empty(t, mini.Keys())
}

func TestPixLabBackchannelErrorShapeAcceptsFastAPIDetail(t *testing.T) {
	t.Parallel()
	err := decodePixLabError(http.StatusUnauthorized, []byte(`{"detail":{"code":"UNAUTHENTICATED","message":"expired"}}`))
	assertWorkbenchError(t, err, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func TestPixLabWorkbenchSessionJSONNeverContainsRawTokens(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(types.PixLabWorkbenchSession{
		Principal: types.PixLabPrincipal{UserID: "u1"}, CSRFHash: strings.Repeat("a", 64),
	})
	require.NoError(t, err)
	require.NotContains(t, string(raw), "session-token")
	require.NotContains(t, string(raw), "csrf-token")
}

func assertWorkbenchError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var target *PixLabWorkbenchError
	require.ErrorAs(t, err, &target)
	require.Equal(t, status, target.Status)
	require.Equal(t, code, target.Code)
}
