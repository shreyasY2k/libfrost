package usermgmt

import (
	"fmt"
	"net/http"

	"github.com/bytedance/sonic"
	"github.com/maximhq/bifrost/core/schemas"
)

// APIHandler provides REST endpoints for workspace and user management.
// These are registered as additional HTTP routes in the Bifrost transport.
type APIHandler struct {
	plugin *UserMgmtPlugin
	logger schemas.Logger
}

// NewAPIHandler creates a new API handler.
func NewAPIHandler(plugin *UserMgmtPlugin) *APIHandler {
	return &APIHandler{
		plugin: plugin,
		logger: plugin.logger,
	}
}

// WorkspaceResponse is the JSON response for workspace endpoints.
type WorkspaceResponse struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	OrgID         string   `json:"org_id"`
	TeamID        *string  `json:"team_id,omitempty"`
	AllowedModels []string `json:"allowed_models,omitempty"`
	IsActive      bool     `json:"is_active"`
}

// CreateWorkspaceRequest is the JSON request for creating a workspace.
type CreateWorkspaceRequest struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	OrgID         string   `json:"org_id"`
	TeamID        *string  `json:"team_id,omitempty"`
	VirtualKeyID  *string  `json:"virtual_key_id,omitempty"`
	AllowedModels []string `json:"allowed_models,omitempty"`
	KeycloakAttr  string   `json:"keycloak_attr,omitempty"`
}

// AssignUserRequest is the JSON request for assigning a user to a workspace.
type AssignUserRequest struct {
	UserID      string  `json:"user_id"`
	Email       string  `json:"email"`
	BudgetID    *string `json:"budget_id,omitempty"`
	RateLimitID *string `json:"rate_limit_id,omitempty"`
}

// HandleListWorkspaces returns workspaces for the authenticated user.
// GET /api/enterprise/workspaces
// Requires: Authorization header with valid JWT
func (h *APIHandler) HandleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID") // Set by auth middleware upstream
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	if h.plugin.wsStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "workspace store not initialized"})
		return
	}

	workspaces := h.plugin.wsStore.GetUserWorkspaces(userID)
	var resp []WorkspaceResponse
	for _, ws := range workspaces {
		models, _ := h.plugin.wsStore.GetAllowedModels(ws.ID)
		resp = append(resp, WorkspaceResponse{
			ID:            ws.ID,
			Name:          ws.Name,
			Description:   ws.Description,
			OrgID:         ws.OrgID,
			TeamID:        ws.TeamID,
			AllowedModels: models,
			IsActive:      ws.IsActive,
		})
	}

	if resp == nil {
		resp = []WorkspaceResponse{}
	}
	writeJSON(w, http.StatusOK, resp)
}

// HandleCreateWorkspace creates a new workspace.
// POST /api/enterprise/workspaces
func (h *APIHandler) HandleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req CreateWorkspaceRequest
	if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	defer r.Body.Close()

	if req.Name == "" || req.OrgID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and org_id are required"})
		return
	}

	var modelsJSON []byte
	if len(req.AllowedModels) > 0 {
		var err error
		modelsJSON, err = sonic.Marshal(req.AllowedModels)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to marshal allowed_models"})
			return
		}
	}

	ws := &TableEnterpriseWorkspace{
		ID:            req.ID,
		Name:          req.Name,
		Description:   req.Description,
		OrgID:         req.OrgID,
		TeamID:        req.TeamID,
		VirtualKeyID:  req.VirtualKeyID,
		AllowedModels: modelsJSON,
		KeycloakAttr:  req.KeycloakAttr,
		IsActive:      true,
	}

	if err := h.plugin.wsStore.CreateWorkspace(r.Context(), ws); err != nil {
		h.logger.Error(fmt.Sprintf("[%s] Failed to create workspace: %v", PluginName, err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create workspace"})
		return
	}

	writeJSON(w, http.StatusCreated, WorkspaceResponse{
		ID:            ws.ID,
		Name:          ws.Name,
		Description:   ws.Description,
		OrgID:         ws.OrgID,
		TeamID:        ws.TeamID,
		AllowedModels: req.AllowedModels,
		IsActive:      ws.IsActive,
	})
}

// HandleAssignUser assigns a user to a workspace.
// POST /api/enterprise/workspaces/{workspaceID}/users
func (h *APIHandler) HandleAssignUser(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	if workspaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "workspace_id is required"})
		return
	}

	var req AssignUserRequest
	if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	defer r.Body.Close()

	if req.UserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}

	wu := &TableEnterpriseWorkspaceUser{
		ID:          fmt.Sprintf("%s-%s", workspaceID, req.UserID),
		WorkspaceID: workspaceID,
		UserID:      req.UserID,
		Email:       req.Email,
		BudgetID:    req.BudgetID,
		RateLimitID: req.RateLimitID,
		IsActive:    true,
	}

	if err := h.plugin.wsStore.AddUserToWorkspace(r.Context(), wu); err != nil {
		h.logger.Error(fmt.Sprintf("[%s] Failed to assign user to workspace: %v", PluginName, err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to assign user"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "assigned"})
}

// HandleRemoveUser removes a user from a workspace.
// DELETE /api/enterprise/workspaces/{workspaceID}/users/{userID}
func (h *APIHandler) HandleRemoveUser(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	userID := r.PathValue("userID")

	if workspaceID == "" || userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "workspace_id and user_id are required"})
		return
	}

	if err := h.plugin.wsStore.RemoveUserFromWorkspace(r.Context(), userID, workspaceID); err != nil {
		h.logger.Error(fmt.Sprintf("[%s] Failed to remove user from workspace: %v", PluginName, err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to remove user"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// HandleDeleteWorkspace deletes a workspace.
// DELETE /api/enterprise/workspaces/{workspaceID}
func (h *APIHandler) HandleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	if workspaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "workspace_id is required"})
		return
	}

	if err := h.plugin.wsStore.DeleteWorkspace(r.Context(), workspaceID); err != nil {
		h.logger.Error(fmt.Sprintf("[%s] Failed to delete workspace: %v", PluginName, err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete workspace"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// HandleUpdateCredits updates a user's budget in a workspace.
// PATCH /api/enterprise/users/{userID}/credits
func (h *APIHandler) HandleUpdateCredits(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}

	var req struct {
		BudgetAmount float64 `json:"budget_amount"`
	}
	if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	defer r.Body.Close()

	if h.plugin.govStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "governance store not initialized"})
		return
	}

	ug, exists := h.plugin.govStore.GetUserGovernance(userID)
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user governance entry not found"})
		return
	}

	if ug.Budget != nil {
		ug.Budget.Amount = req.BudgetAmount
		h.plugin.govStore.UpdateUserGovernanceInMemory(userID, ug.Budget, ug.RateLimit)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":       userID,
		"budget_amount": req.BudgetAmount,
		"status":        "updated",
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	sonic.ConfigDefault.NewEncoder(w).Encode(data)
}
