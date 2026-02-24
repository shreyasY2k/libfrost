// Package usermgmt provides enterprise user management for Bifrost.
// It validates Keycloak JWTs, resolves workspace context, and enforces model access.
package usermgmt

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore"
	configstoreTables "github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/maximhq/bifrost/plugins/governance"
	"gorm.io/gorm"
)

const PluginName = "usermgmt"

// Context keys for enterprise metadata
const (
	ContextKeyWorkspaceID   schemas.BifrostContextKey = "enterprise-workspace-id"
	ContextKeyWorkspaceName schemas.BifrostContextKey = "enterprise-workspace-name"
	ContextKeyOrgID         schemas.BifrostContextKey = "enterprise-org-id"
	ContextKeyOrgName       schemas.BifrostContextKey = "enterprise-org-name"
	ContextKeyTeamID        schemas.BifrostContextKey = "enterprise-team-id"
	ContextKeyUserEmail     schemas.BifrostContextKey = "enterprise-user-email"
)

// Config holds the plugin configuration loaded from bifrost config.json.
type Config struct {
	KeycloakBaseURL       string  `json:"keycloak_base_url"`
	KeycloakRealm         string  `json:"keycloak_realm"`
	KeycloakAdminClientID string  `json:"keycloak_admin_client_id"`
	KeycloakAdminSecret   string  `json:"keycloak_admin_client_secret"`
	JWKSURL               string  `json:"jwks_url"`
	JWTIssuer             string  `json:"jwt_issuer"`
	DefaultUserBudget     float64 `json:"default_user_budget"`
	SyncIntervalSeconds   int     `json:"sync_interval_seconds"`
}

// UserMgmtPlugin implements HTTPTransportPlugin and LLMPlugin for enterprise user management.
type UserMgmtPlugin struct {
	ctx        context.Context
	cancelFunc context.CancelFunc
	wg         sync.WaitGroup
	cleanupOnce sync.Once

	config     *Config
	logger     schemas.Logger
	validator  *JWTValidator
	kcClient   *KeycloakClient
	wsStore    *WorkspaceStore
	govStore   governance.GovernanceStore
	configStore configstore.ConfigStore

	defaultBudget float64
}

// Init initializes the UserMgmt plugin.
func Init(
	ctx context.Context,
	config *Config,
	logger schemas.Logger,
	configStore configstore.ConfigStore,
	db *gorm.DB,
	govStore governance.GovernanceStore,
) (*UserMgmtPlugin, error) {
	if config == nil {
		return nil, fmt.Errorf("usermgmt plugin requires config")
	}
	if config.JWKSURL == "" || config.JWTIssuer == "" {
		return nil, fmt.Errorf("usermgmt plugin requires jwks_url and jwt_issuer in config")
	}

	pluginCtx, cancel := context.WithCancel(ctx)

	plugin := &UserMgmtPlugin{
		ctx:         pluginCtx,
		cancelFunc:  cancel,
		config:      config,
		logger:      logger,
		configStore: configStore,
		govStore:    govStore,
		defaultBudget: config.DefaultUserBudget,
	}

	if plugin.defaultBudget <= 0 {
		plugin.defaultBudget = 10.0
	}

	// Initialize JWT validator
	plugin.validator = NewJWTValidator(config.JWKSURL, config.JWTIssuer)

	// Initialize Keycloak admin client
	if config.KeycloakBaseURL != "" && config.KeycloakAdminClientID != "" {
		plugin.kcClient = NewKeycloakClient(
			config.KeycloakBaseURL,
			config.KeycloakRealm,
			config.KeycloakAdminClientID,
			config.KeycloakAdminSecret,
			logger,
		)
	}

	// Initialize workspace store
	if db != nil {
		wsStore, err := NewWorkspaceStore(db, logger)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to initialize workspace store: %w", err)
		}
		plugin.wsStore = wsStore
	}

	// Start KC sync worker if configured
	if plugin.kcClient != nil && config.SyncIntervalSeconds > 0 {
		plugin.startSyncWorker()
	}

	logger.Info(fmt.Sprintf("[%s] Plugin initialized (issuer=%s)", PluginName, config.JWTIssuer))
	return plugin, nil
}

// GetName returns the plugin name.
func (p *UserMgmtPlugin) GetName() string {
	return PluginName
}

// Cleanup stops background workers and releases resources.
func (p *UserMgmtPlugin) Cleanup() error {
	p.cleanupOnce.Do(func() {
		p.cancelFunc()
		p.wg.Wait()
		p.logger.Info(fmt.Sprintf("[%s] Plugin cleaned up", PluginName))
	})
	return nil
}

// HTTPTransportPreHook validates JWT, resolves workspace, and sets governance context.
func (p *UserMgmtPlugin) HTTPTransportPreHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest) (*schemas.HTTPResponse, error) {
	// Extract Bearer token from Authorization header
	authHeader := req.CaseInsensitiveHeaderLookup("Authorization")
	if authHeader == "" {
		return unauthorizedResponse("missing Authorization header"), nil
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return unauthorizedResponse("invalid Authorization header format"), nil
	}
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	// Validate JWT
	claims, err := p.validator.Validate(p.ctx, tokenString)
	if err != nil {
		p.logger.Warn(fmt.Sprintf("[%s] JWT validation failed: %v", PluginName, err))
		return unauthorizedResponse("invalid or expired token"), nil
	}

	// Set governance user ID from JWT subject
	ctx.SetValue(schemas.BifrostContextKeyGovernanceUserID, claims.Subject)
	ctx.SetValue(ContextKeyUserEmail, claims.Email)

	// Set organization context (maps to governance customer)
	if claims.OrgID != "" {
		ctx.SetValue(schemas.BifrostContextKeyGovernanceCustomerID, claims.OrgID)
		ctx.SetValue(schemas.BifrostContextKeyGovernanceCustomerName, claims.OrgName)
		ctx.SetValue(ContextKeyOrgID, claims.OrgID)
		ctx.SetValue(ContextKeyOrgName, claims.OrgName)
	}

	// Set team context from first group (maps to governance team)
	if len(claims.Groups) > 0 {
		ctx.SetValue(schemas.BifrostContextKeyGovernanceTeamID, claims.Groups[0])
		ctx.SetValue(schemas.BifrostContextKeyGovernanceTeamName, claims.Groups[0])
		ctx.SetValue(ContextKeyTeamID, claims.Groups[0])
	}

	// Resolve workspace
	workspaceID := req.CaseInsensitiveHeaderLookup("X-Workspace-ID")
	if workspaceID == "" {
		// Fall back to JWT workspace claim
		workspaceID = claims.Workspace
	}

	if workspaceID != "" && p.wsStore != nil {
		// Validate user has access to workspace
		_, err := p.wsStore.ValidateUserWorkspaceAccess(claims.Subject, workspaceID)
		if err != nil {
			p.logger.Warn(fmt.Sprintf("[%s] Workspace access denied: %v", PluginName, err))
			return forbiddenResponse("access denied to workspace"), nil
		}

		ws, ok := p.wsStore.GetWorkspace(workspaceID)
		if !ok {
			return forbiddenResponse("workspace not found"), nil
		}

		ctx.SetValue(ContextKeyWorkspaceID, workspaceID)
		ctx.SetValue(ContextKeyWorkspaceName, ws.Name)

		// Set workspace's virtual key for model routing
		if ws.VirtualKeyID != nil && *ws.VirtualKeyID != "" {
			ctx.SetValue(schemas.BifrostContextKeyVirtualKey, *ws.VirtualKeyID)
		}
	}

	// Auto-create UserGovernance entry on first request if enterprise governance is enabled
	if p.govStore != nil {
		if _, exists := p.govStore.GetUserGovernance(claims.Subject); !exists {
			p.autoCreateUserGovernance(claims.Subject)
		}
	}

	// Mark as enterprise request
	ctx.SetValue(schemas.BifrostContextKeyIsEnterprise, true)

	return nil, nil
}

// HTTPTransportPostHook is a no-op for this plugin.
func (p *UserMgmtPlugin) HTTPTransportPostHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest, resp *schemas.HTTPResponse) error {
	return nil
}

// HTTPTransportStreamChunkHook is a no-op for this plugin.
func (p *UserMgmtPlugin) HTTPTransportStreamChunkHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest, chunk *schemas.BifrostStreamChunk) (*schemas.BifrostStreamChunk, error) {
	return chunk, nil
}

// PreLLMHook checks if the requested model is allowed in the user's workspace.
func (p *UserMgmtPlugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	workspaceIDVal := ctx.Value(ContextKeyWorkspaceID)
	if workspaceIDVal == nil || p.wsStore == nil {
		return req, nil, nil
	}
	workspaceID, ok := workspaceIDVal.(string)
	if !ok || workspaceID == "" {
		return req, nil, nil
	}

	// Check model access
	model := req.Model
	allowed, err := p.wsStore.IsModelAllowed(workspaceID, model)
	if err != nil {
		p.logger.Warn(fmt.Sprintf("[%s] Model access check failed: %v", PluginName, err))
		return req, nil, nil // Fail open
	}

	if !allowed {
		return nil, nil, &schemas.BifrostError{
			StatusCode: 403,
			Message:    fmt.Sprintf("model %s is not allowed in this workspace", model),
			Type:       "workspace_model_access_denied",
		}
	}

	return req, nil, nil
}

// PostLLMHook is a no-op for this plugin.
func (p *UserMgmtPlugin) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, bifrostErr, nil
}

// autoCreateUserGovernance creates a default UserGovernance entry with budget for a new user.
func (p *UserMgmtPlugin) autoCreateUserGovernance(userID string) {
	budget := &configstoreTables.TableBudget{
		ID:            fmt.Sprintf("user-budget-%s", userID),
		Amount:        p.defaultBudget,
		ResetDuration: stringPtr("1M"),
		IsActive:      true,
	}

	p.govStore.CreateUserGovernanceInMemory(userID, budget, nil)
	p.logger.Info(fmt.Sprintf("[%s] Auto-created governance for user %s (budget=$%.2f/month)", PluginName, userID, p.defaultBudget))
}

// Helper: build 401 response
func unauthorizedResponse(message string) *schemas.HTTPResponse {
	resp := schemas.AcquireHTTPResponse()
	resp.StatusCode = 401
	resp.Headers["Content-Type"] = "application/json"
	body, _ := sonic.Marshal(map[string]string{
		"error":   "unauthorized",
		"message": message,
	})
	resp.Body = body
	return resp
}

// Helper: build 403 response
func forbiddenResponse(message string) *schemas.HTTPResponse {
	resp := schemas.AcquireHTTPResponse()
	resp.StatusCode = 403
	resp.Headers["Content-Type"] = "application/json"
	body, _ := sonic.Marshal(map[string]string{
		"error":   "forbidden",
		"message": message,
	})
	resp.Body = body
	return resp
}

func stringPtr(s string) *string {
	return &s
}
