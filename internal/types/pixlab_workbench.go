package types

import "time"

const (
	PixLabBindingStatusActive = "active"
	PixLabCapabilityRead      = "read"
	PixLabCapabilityUpload    = "upload"
	PixLabCapabilityChat      = "chat"
)

// PixLabProjectBinding maps one PixLab project to a server-controlled WeKnora scope.
// Browser requests never choose these IDs.
type PixLabProjectBinding struct {
	ProjectCode     string    `json:"project_code"      gorm:"type:varchar(64);primaryKey"`
	ProjectName     string    `json:"project_name"      gorm:"type:varchar(255);not null;default:''"`
	TenantID        uint64    `json:"tenant_id"         gorm:"not null;index:idx_pixlab_project_bindings_tenant"`
	KnowledgeBaseID string    `json:"knowledge_base_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	AgentID         string    `json:"agent_id"          gorm:"type:varchar(64);not null"`
	Status          string    `json:"status"            gorm:"type:varchar(24);not null;index:idx_pixlab_project_bindings_status"`
	Revision        int       `json:"revision"          gorm:"not null;default:1"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (PixLabProjectBinding) TableName() string { return "pixlab_project_bindings" }

// PixLabPrincipal is the authorization snapshot issued and continuously
// revalidated by PixLab. It deliberately contains no WeKnora resource IDs.
type PixLabPrincipal struct {
	UserID            string   `json:"user_id"`
	UID               string   `json:"uid,omitempty"`
	DisplayName       string   `json:"display_name,omitempty"`
	ProjectCode       string   `json:"project_code"`
	ProjectName       string   `json:"project_name,omitempty"`
	PixLabSessionID   string   `json:"pixlab_session_id"`
	PermissionVersion int      `json:"permission_version"`
	BindingRevision   int      `json:"binding_revision"`
	Capabilities      []string `json:"capabilities"`
}

func (p PixLabPrincipal) HasCapability(capability string) bool {
	for _, current := range p.Capabilities {
		if current == capability {
			return true
		}
	}
	return false
}

// PixLabWorkbenchSession is stored only in Redis. The cookie contains a random
// lookup token and the browser receives a separate CSRF token.
type PixLabWorkbenchSession struct {
	Principal PixLabPrincipal `json:"principal"`
	CSRFHash  string          `json:"csrf_hash"`
	CreatedAt time.Time       `json:"created_at"`
}
