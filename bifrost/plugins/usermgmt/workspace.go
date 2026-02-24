package usermgmt

import (
	"context"
	"fmt"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/maximhq/bifrost/core/schemas"
	"gorm.io/gorm"
)

// WorkspaceStore manages workspace data with an in-memory cache backed by PostgreSQL.
type WorkspaceStore struct {
	db         *gorm.DB
	logger     schemas.Logger
	workspaces sync.Map // id -> *TableEnterpriseWorkspace
	userAccess sync.Map // "userID:workspaceID" -> *TableEnterpriseWorkspaceUser
}

// NewWorkspaceStore creates a new workspace store.
func NewWorkspaceStore(db *gorm.DB, logger schemas.Logger) (*WorkspaceStore, error) {
	if err := AutoMigrate(db); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate enterprise tables: %w", err)
	}

	store := &WorkspaceStore{
		db:     db,
		logger: logger,
	}

	if err := store.loadAll(); err != nil {
		return nil, fmt.Errorf("failed to load workspace data: %w", err)
	}

	return store, nil
}

// loadAll loads all workspaces and user-workspace mappings into memory.
func (s *WorkspaceStore) loadAll() error {
	var workspaces []TableEnterpriseWorkspace
	if err := s.db.Where("is_active = ?", true).Find(&workspaces).Error; err != nil {
		return err
	}
	for i := range workspaces {
		s.workspaces.Store(workspaces[i].ID, &workspaces[i])
	}

	var users []TableEnterpriseWorkspaceUser
	if err := s.db.Where("is_active = ?", true).Find(&users).Error; err != nil {
		return err
	}
	for i := range users {
		key := users[i].UserID + ":" + users[i].WorkspaceID
		s.userAccess.Store(key, &users[i])
	}

	s.logger.Info(fmt.Sprintf("[usermgmt] Loaded %d workspaces and %d user-workspace mappings", len(workspaces), len(users)))
	return nil
}

// GetWorkspace returns a workspace by ID.
func (s *WorkspaceStore) GetWorkspace(id string) (*TableEnterpriseWorkspace, bool) {
	v, ok := s.workspaces.Load(id)
	if !ok {
		return nil, false
	}
	return v.(*TableEnterpriseWorkspace), true
}

// GetUserWorkspaces returns all workspaces accessible by a user.
func (s *WorkspaceStore) GetUserWorkspaces(userID string) []*TableEnterpriseWorkspace {
	var result []*TableEnterpriseWorkspace
	s.userAccess.Range(func(key, value any) bool {
		wu := value.(*TableEnterpriseWorkspaceUser)
		if wu.UserID == userID && wu.IsActive {
			if ws, ok := s.GetWorkspace(wu.WorkspaceID); ok {
				result = append(result, ws)
			}
		}
		return true
	})
	return result
}

// ValidateUserWorkspaceAccess checks if a user has access to a specific workspace.
func (s *WorkspaceStore) ValidateUserWorkspaceAccess(userID, workspaceID string) (*TableEnterpriseWorkspaceUser, error) {
	key := userID + ":" + workspaceID
	v, ok := s.userAccess.Load(key)
	if !ok {
		return nil, fmt.Errorf("user %s does not have access to workspace %s", userID, workspaceID)
	}
	wu := v.(*TableEnterpriseWorkspaceUser)
	if !wu.IsActive {
		return nil, fmt.Errorf("user %s access to workspace %s is disabled", userID, workspaceID)
	}
	return wu, nil
}

// GetAllowedModels returns the allowed models for a workspace.
func (s *WorkspaceStore) GetAllowedModels(workspaceID string) ([]string, error) {
	ws, ok := s.GetWorkspace(workspaceID)
	if !ok {
		return nil, fmt.Errorf("workspace %s not found", workspaceID)
	}
	if len(ws.AllowedModels) == 0 {
		return nil, nil // No restrictions
	}
	var models []string
	if err := sonic.Unmarshal(ws.AllowedModels, &models); err != nil {
		return nil, fmt.Errorf("failed to parse allowed_models for workspace %s: %w", workspaceID, err)
	}
	return models, nil
}

// IsModelAllowed checks if a model is allowed in a workspace.
func (s *WorkspaceStore) IsModelAllowed(workspaceID, model string) (bool, error) {
	models, err := s.GetAllowedModels(workspaceID)
	if err != nil {
		return false, err
	}
	if models == nil {
		return true, nil // No restrictions
	}
	for _, m := range models {
		if m == model {
			return true, nil
		}
	}
	return false, nil
}

// CreateWorkspace creates a new workspace in both DB and cache.
func (s *WorkspaceStore) CreateWorkspace(ctx context.Context, ws *TableEnterpriseWorkspace) error {
	if err := s.db.WithContext(ctx).Create(ws).Error; err != nil {
		return err
	}
	s.workspaces.Store(ws.ID, ws)
	return nil
}

// UpdateWorkspace updates a workspace in both DB and cache.
func (s *WorkspaceStore) UpdateWorkspace(ctx context.Context, ws *TableEnterpriseWorkspace) error {
	if err := s.db.WithContext(ctx).Save(ws).Error; err != nil {
		return err
	}
	s.workspaces.Store(ws.ID, ws)
	return nil
}

// DeleteWorkspace soft-deletes a workspace.
func (s *WorkspaceStore) DeleteWorkspace(ctx context.Context, id string) error {
	if err := s.db.WithContext(ctx).Model(&TableEnterpriseWorkspace{}).Where("id = ?", id).Update("is_active", false).Error; err != nil {
		return err
	}
	s.workspaces.Delete(id)
	return nil
}

// AddUserToWorkspace adds a user to a workspace.
func (s *WorkspaceStore) AddUserToWorkspace(ctx context.Context, wu *TableEnterpriseWorkspaceUser) error {
	if err := s.db.WithContext(ctx).Create(wu).Error; err != nil {
		return err
	}
	key := wu.UserID + ":" + wu.WorkspaceID
	s.userAccess.Store(key, wu)
	return nil
}

// RemoveUserFromWorkspace removes a user from a workspace.
func (s *WorkspaceStore) RemoveUserFromWorkspace(ctx context.Context, userID, workspaceID string) error {
	if err := s.db.WithContext(ctx).Model(&TableEnterpriseWorkspaceUser{}).
		Where("user_id = ? AND workspace_id = ?", userID, workspaceID).
		Update("is_active", false).Error; err != nil {
		return err
	}
	key := userID + ":" + workspaceID
	s.userAccess.Delete(key)
	return nil
}
