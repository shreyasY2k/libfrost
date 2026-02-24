package usermgmt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/maximhq/bifrost/core/schemas"
)

// KeycloakClient communicates with the Keycloak Admin REST API.
type KeycloakClient struct {
	baseURL      string
	realm        string
	clientID     string
	clientSecret string
	logger       schemas.Logger

	// Service account token cache
	tokenMu    sync.RWMutex
	token      string
	tokenExpAt time.Time
}

// KCOrganization represents a Keycloak organization.
type KCOrganization struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// KCGroup represents a Keycloak group.
type KCGroup struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	SubGroups []KCGroup `json:"subGroups,omitempty"`
}

// KCUser represents a Keycloak user.
type KCUser struct {
	ID         string            `json:"id"`
	Username   string            `json:"username"`
	Email      string            `json:"email"`
	Enabled    bool              `json:"enabled"`
	FirstName  string            `json:"firstName"`
	LastName   string            `json:"lastName"`
	Attributes map[string][]string `json:"attributes,omitempty"`
}

// NewKeycloakClient creates a new Keycloak Admin API client.
func NewKeycloakClient(baseURL, realm, clientID, clientSecret string, logger schemas.Logger) *KeycloakClient {
	return &KeycloakClient{
		baseURL:      strings.TrimRight(baseURL, "/"),
		realm:        realm,
		clientID:     clientID,
		clientSecret: clientSecret,
		logger:       logger,
	}
}

// getServiceAccountToken obtains a token using client_credentials grant.
func (kc *KeycloakClient) getServiceAccountToken(ctx context.Context) (string, error) {
	kc.tokenMu.RLock()
	if kc.token != "" && time.Now().Before(kc.tokenExpAt) {
		t := kc.token
		kc.tokenMu.RUnlock()
		return t, nil
	}
	kc.tokenMu.RUnlock()

	kc.tokenMu.Lock()
	defer kc.tokenMu.Unlock()

	// Double-check after acquiring write lock
	if kc.token != "" && time.Now().Before(kc.tokenExpAt) {
		return kc.token, nil
	}

	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", kc.baseURL, kc.realm)
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {kc.clientID},
		"client_secret": {kc.clientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token request returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	kc.token = tokenResp.AccessToken
	// Refresh 30 seconds before expiry
	kc.tokenExpAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn-30) * time.Second)
	return kc.token, nil
}

// adminRequest makes an authenticated request to the Keycloak Admin API.
func (kc *KeycloakClient) adminRequest(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	token, err := kc.getServiceAccountToken(ctx)
	if err != nil {
		return nil, err
	}

	fullURL := fmt.Sprintf("%s/admin/realms/%s%s", kc.baseURL, kc.realm, path)
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("keycloak API %s %s returned %d: %s", method, path, resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// ListOrganizations returns all organizations in the realm.
func (kc *KeycloakClient) ListOrganizations(ctx context.Context) ([]KCOrganization, error) {
	data, err := kc.adminRequest(ctx, http.MethodGet, "/organizations", nil)
	if err != nil {
		return nil, err
	}
	var orgs []KCOrganization
	if err := sonic.Unmarshal(data, &orgs); err != nil {
		return nil, err
	}
	return orgs, nil
}

// ListGroups returns all groups in the realm.
func (kc *KeycloakClient) ListGroups(ctx context.Context) ([]KCGroup, error) {
	data, err := kc.adminRequest(ctx, http.MethodGet, "/groups", nil)
	if err != nil {
		return nil, err
	}
	var groups []KCGroup
	if err := sonic.Unmarshal(data, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// ListOrgMembers returns all members of an organization.
func (kc *KeycloakClient) ListOrgMembers(ctx context.Context, orgID string) ([]KCUser, error) {
	data, err := kc.adminRequest(ctx, http.MethodGet, fmt.Sprintf("/organizations/%s/members", orgID), nil)
	if err != nil {
		return nil, err
	}
	var users []KCUser
	if err := sonic.Unmarshal(data, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// ListGroupMembers returns all members of a group.
func (kc *KeycloakClient) ListGroupMembers(ctx context.Context, groupID string) ([]KCUser, error) {
	data, err := kc.adminRequest(ctx, http.MethodGet, fmt.Sprintf("/groups/%s/members", groupID), nil)
	if err != nil {
		return nil, err
	}
	var users []KCUser
	if err := sonic.Unmarshal(data, &users); err != nil {
		return nil, err
	}
	return users, nil
}
