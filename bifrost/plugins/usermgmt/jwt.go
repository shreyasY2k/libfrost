package usermgmt

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// JWTValidator validates Keycloak JWTs using JWKS.
type JWTValidator struct {
	jwksURL  string
	issuer   string
	keySet   jwk.Set
	mu       sync.RWMutex
	lastFetch time.Time
	cacheTTL  time.Duration
}

// Claims extracted from a validated JWT.
type Claims struct {
	Subject      string            `json:"sub"`
	Email        string            `json:"email"`
	Groups       []string          `json:"groups"`
	Organization map[string]any    `json:"organization"`
	Workspace    string            `json:"workspace"`
	OrgID        string            // resolved from organization claim
	OrgName      string            // resolved from organization claim
}

// NewJWTValidator creates a new JWKS-based JWT validator.
func NewJWTValidator(jwksURL, issuer string) *JWTValidator {
	return &JWTValidator{
		jwksURL:  jwksURL,
		issuer:   issuer,
		cacheTTL: 5 * time.Minute,
	}
}

// fetchKeySet retrieves the JWKS from Keycloak with caching.
func (v *JWTValidator) fetchKeySet(ctx context.Context) (jwk.Set, error) {
	v.mu.RLock()
	if v.keySet != nil && time.Since(v.lastFetch) < v.cacheTTL {
		ks := v.keySet
		v.mu.RUnlock()
		return ks, nil
	}
	v.mu.RUnlock()

	v.mu.Lock()
	defer v.mu.Unlock()

	// Double-check after acquiring write lock
	if v.keySet != nil && time.Since(v.lastFetch) < v.cacheTTL {
		return v.keySet, nil
	}

	ks, err := jwk.Fetch(ctx, v.jwksURL)
	if err != nil {
		// Return stale keys if available
		if v.keySet != nil {
			return v.keySet, nil
		}
		return nil, fmt.Errorf("failed to fetch JWKS from %s: %w", v.jwksURL, err)
	}

	v.keySet = ks
	v.lastFetch = time.Now()
	return ks, nil
}

// Validate validates a JWT token string and extracts claims.
func (v *JWTValidator) Validate(ctx context.Context, tokenString string) (*Claims, error) {
	keySet, err := v.fetchKeySet(ctx)
	if err != nil {
		return nil, fmt.Errorf("JWKS fetch failed: %w", err)
	}

	// Parse and validate the token
	token, err := jwt.Parse(
		[]byte(tokenString),
		jwt.WithKeySet(keySet, jws.WithInferAlgorithmFromKey(true)),
		jwt.WithIssuer(v.issuer),
		jwt.WithValidate(true),
	)
	if err != nil {
		return nil, fmt.Errorf("JWT validation failed: %w", err)
	}

	claims := &Claims{
		Subject: token.Subject(),
	}

	// Extract email
	if email, ok := token.Get("email"); ok {
		if emailStr, ok := email.(string); ok {
			claims.Email = emailStr
		}
	}

	// Extract groups
	if groups, ok := token.Get("groups"); ok {
		if groupSlice, ok := groups.([]interface{}); ok {
			for _, g := range groupSlice {
				if gs, ok := g.(string); ok {
					claims.Groups = append(claims.Groups, gs)
				}
			}
		}
	}

	// Extract organization claim (KC26 format: map of org_id -> {name: ...})
	if org, ok := token.Get("organization"); ok {
		if orgMap, ok := org.(map[string]interface{}); ok {
			claims.Organization = orgMap
			// Extract the first (primary) organization
			for orgID, orgData := range orgMap {
				claims.OrgID = orgID
				if orgDataMap, ok := orgData.(map[string]interface{}); ok {
					if name, ok := orgDataMap["name"].(string); ok {
						claims.OrgName = name
					}
				}
				break // Use first organization as primary
			}
		}
	}

	// Extract workspace attribute
	if ws, ok := token.Get("workspace"); ok {
		if wsStr, ok := ws.(string); ok {
			claims.Workspace = wsStr
		}
	}

	return claims, nil
}
