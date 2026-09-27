package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"gopkg.in/yaml.v3"
)

type contextKey int

const identityKey contextKey = 1

type (
	// Identity is the authenticated caller.
	Identity struct {
		Sub    string
		Email  string
		Name   string
		Groups []string
	}

	// IdentitySource resolves caller identity from an HTTP request.
	IdentitySource interface {
		FromRequest(r *http.Request) (Identity, error)
	}

	// DisabledIdentity always succeeds (local stub).
	DisabledIdentity struct{}

	// StaticIdentity returns a fixed identity.
	StaticIdentity struct {
		Identity Identity
	}
)

// Provide builds IdentitySource from YAML auth.mode.
func Provide(rawConfig []byte) (IdentitySource, error) {
	var cf struct {
		Auth struct {
			Mode   string `yaml:"mode"`
			Static struct {
				Sub    string   `yaml:"sub"`
				Email  string   `yaml:"email"`
				Name   string   `yaml:"name"`
				Groups []string `yaml:"groups"`
			} `yaml:"static"`
		} `yaml:"auth"`
	}
	if err := yaml.Unmarshal(rawConfig, &cf); err != nil {
		return nil, fmt.Errorf("auth config: %w", err)
	}
	mode := strings.ToLower(strings.TrimSpace(cf.Auth.Mode))
	if mode == "" {
		mode = "disabled"
	}
	switch mode {
	case "static":
		id := Identity{
			Sub:    cf.Auth.Static.Sub,
			Email:  cf.Auth.Static.Email,
			Name:   cf.Auth.Static.Name,
			Groups: cf.Auth.Static.Groups,
		}
		if id.Sub == "" {
			id.Sub = "local"
		}
		if id.Email == "" {
			id.Email = "local@dbt-gate"
		}
		return StaticIdentity{Identity: id}, nil
	default:
		return DisabledIdentity{}, nil
	}
}

// FromRequest implements IdentitySource.
func (DisabledIdentity) FromRequest(*http.Request) (Identity, error) {
	return Identity{Sub: "anonymous", Email: "anonymous@local", Name: "anonymous"}, nil
}

// FromRequest implements IdentitySource.
func (v StaticIdentity) FromRequest(*http.Request) (Identity, error) {
	if v.Identity.Sub == "" && v.Identity.Email == "" {
		return Identity{}, fmt.Errorf("auth: empty static identity")
	}
	return v.Identity, nil
}

// WithIdentity stores identity in context.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// IdentityFromContext returns the authenticated identity.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey).(Identity)
	return id, ok
}
