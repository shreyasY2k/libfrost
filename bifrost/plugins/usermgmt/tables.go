package usermgmt

import (
	"time"

	"gorm.io/gorm"
)

// TableEnterpriseWorkspace represents a workspace within an organization/team hierarchy.
// Each workspace owns a virtual key for model routing and has its own allowed models list.
type TableEnterpriseWorkspace struct {
	ID            string  `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name          string  `gorm:"not null;type:varchar(255)" json:"name"`
	Description   string  `gorm:"type:text" json:"description,omitempty"`
	OrgID         string  `gorm:"index;type:varchar(255);not null" json:"org_id"`         // FK to governance_customers
	TeamID        *string `gorm:"index;type:varchar(255)" json:"team_id,omitempty"`       // FK to governance_teams (optional)
	VirtualKeyID  *string `gorm:"type:varchar(255)" json:"virtual_key_id,omitempty"`      // FK to governance_virtual_keys
	AllowedModels []byte  `gorm:"type:jsonb" json:"allowed_models"`                       // JSON array of allowed model strings
	BudgetID      *string `gorm:"type:varchar(255)" json:"budget_id,omitempty"`           // workspace-level budget
	RateLimitID   *string `gorm:"type:varchar(255)" json:"rate_limit_id,omitempty"`       // workspace-level rate limit
	KeycloakAttr  string  `gorm:"type:varchar(255)" json:"keycloak_attr,omitempty"`       // KC user attribute value for this workspace
	IsActive      bool    `gorm:"not null;default:true" json:"is_active"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (TableEnterpriseWorkspace) TableName() string {
	return "enterprise_workspaces"
}

// TableEnterpriseWorkspaceUser represents a user's membership in a workspace.
type TableEnterpriseWorkspaceUser struct {
	ID          string  `gorm:"primaryKey;type:varchar(36)" json:"id"`
	WorkspaceID string  `gorm:"index;not null;type:varchar(36)" json:"workspace_id"`       // FK to enterprise_workspaces
	UserID      string  `gorm:"index;not null;type:varchar(255)" json:"user_id"`            // Keycloak subject ID
	Email       string  `gorm:"type:varchar(255)" json:"email"`
	BudgetID    *string `gorm:"type:varchar(255)" json:"budget_id,omitempty"`               // user-specific budget override
	RateLimitID *string `gorm:"type:varchar(255)" json:"rate_limit_id,omitempty"`           // user-specific rate limit override
	IsActive    bool    `gorm:"not null;default:true" json:"is_active"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (TableEnterpriseWorkspaceUser) TableName() string {
	return "enterprise_workspace_users"
}

// AutoMigrate creates/updates the enterprise tables in the database.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&TableEnterpriseWorkspace{},
		&TableEnterpriseWorkspaceUser{},
	)
}
