package gate

import (
	"context"
	"errors"
	"log/slog"

	"github.com/barloc/dbt-gate/app/index"
)

const apiVersion = 1

type (
	// Service exposes catalog tools over the in-memory index.
	Service struct {
		store *index.Store
		log   *slog.Logger
	}

	// Envelope is the public JSON response shape plus index meta.
	Envelope struct {
		OK                  bool    `json:"ok"`
		APIVersion          int     `json:"api_version"`
		Product             string  `json:"product,omitempty"`
		Checksum            string  `json:"checksum,omitempty"`
		ManifestGeneratedAt string  `json:"manifest_generated_at,omitempty"`
		Data                any     `json:"data"`
		Paging              *Paging `json:"paging"`
		Error               *Error  `json:"error"`
	}

	// Paging is unused in v1 search (fixed top-N) but kept for envelope parity.
	Paging struct {
		HasMore bool `json:"has_more"`
	}

	// Error is a structured API error.
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message,omitempty"`
	}

	SearchRequest struct {
		Query        string `json:"query"`
		ResourceType string `json:"resource_type"`
		Limit        int    `json:"limit"`
	}

	GetModelRequest struct {
		FQN      string `json:"fqn"`
		UniqueID string `json:"unique_id"`
		ID       string `json:"id"` // alias: fqn or unique_id
	}

	LineageRequest struct {
		FQN      string `json:"fqn"`
		UniqueID string `json:"unique_id"`
		ID       string `json:"id"`
		Depth    int    `json:"depth"`
	}

	GetExposureRequest struct {
		UniqueID string `json:"unique_id"`
		Name     string `json:"name"`
		ID       string `json:"id"`
	}
)

// NewService constructs the gate Service.
func NewService(store *index.Store, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store: store,
		log:   log.With(slog.String("component", "gate")),
	}
}

func (s *Service) allow() index.Allowlist {
	return index.Allowlist{All: true}
}

func (s *Service) withMeta(env Envelope) Envelope {
	idx := s.store.Get()
	env.APIVersion = apiVersion
	if idx != nil {
		env.Product = idx.Meta.Product
		env.Checksum = idx.Meta.Checksum
		env.ManifestGeneratedAt = idx.Meta.ManifestGeneratedAt
	} else {
		env.Product = s.store.Product()
	}
	return env
}

func (s *Service) unavailable() Envelope {
	return s.withMeta(Envelope{
		OK:    false,
		Error: &Error{Code: "index_unavailable", Message: "manifest index not loaded yet"},
	})
}

func (s *Service) ok(data any) Envelope {
	return s.withMeta(Envelope{OK: true, Data: data, Paging: &Paging{HasMore: false}})
}

func (s *Service) fail(code, msg string) Envelope {
	return s.withMeta(Envelope{OK: false, Error: &Error{Code: code, Message: msg}})
}

// Search handles POST /v1/search.
func (s *Service) Search(_ context.Context, req SearchRequest) Envelope {
	idx := s.store.Get()
	if idx == nil {
		return s.unavailable()
	}
	limit := req.Limit
	if limit <= 0 {
		limit = s.store.MaxHits()
	}
	hits, err := idx.Search(index.SearchRequest{
		Query:        req.Query,
		ResourceType: req.ResourceType,
		Limit:        limit,
		Allow:        s.allow(),
	})
	if err != nil {
		if errors.Is(err, index.ErrInvalidQuery) {
			return s.fail("invalid_argument", err.Error())
		}
		return s.fail("internal", err.Error())
	}
	return s.ok(map[string]any{"hits": hits, "count": len(hits)})
}

// GetModel handles POST /v1/get_model.
func (s *Service) GetModel(_ context.Context, req GetModelRequest) Envelope {
	idx := s.store.Get()
	if idx == nil {
		return s.unavailable()
	}
	id := first(req.ID, req.FQN, req.UniqueID)
	n, err := idx.GetModel(id, s.allow())
	if err != nil {
		return s.mapErr(err)
	}
	return s.ok(map[string]any{
		"unique_id":     n.UniqueID,
		"resource_type": n.ResourceType,
		"fqn":           n.FQN,
		"name":          n.Name,
		"alias":         n.Alias,
		"catalog":       n.Catalog,
		"schema":        n.Schema,
		"path":          n.Path,
		"description":   n.Description,
		"tags":          n.Tags,
		"columns":       n.Columns,
		"compiled_sql":  n.CompiledSQL,
	})
}

// Lineage handles POST /v1/lineage.
func (s *Service) Lineage(_ context.Context, req LineageRequest) Envelope {
	idx := s.store.Get()
	if idx == nil {
		return s.unavailable()
	}
	id := first(req.ID, req.FQN, req.UniqueID)
	res, err := idx.Lineage(id, req.Depth, s.allow())
	if err != nil {
		return s.mapErr(err)
	}
	return s.ok(res)
}

// GetExposure handles POST /v1/get_exposure.
func (s *Service) GetExposure(_ context.Context, req GetExposureRequest) Envelope {
	idx := s.store.Get()
	if idx == nil {
		return s.unavailable()
	}
	id := first(req.ID, req.UniqueID, req.Name)
	e, err := idx.GetExposure(id)
	if err != nil {
		return s.mapErr(err)
	}
	depends := make([]any, 0, len(e.DependsOn))
	for _, d := range e.DependsOn {
		if n, ok := idx.ByUID[d]; ok {
			depends = append(depends, map[string]any{
				"unique_id":     n.UniqueID,
				"fqn":           n.FQN,
				"resource_type": n.ResourceType,
				"hidden":        false,
			})
			continue
		}
		if stub, ok := idx.Stubs[d]; ok {
			depends = append(depends, map[string]any{
				"unique_id":     stub.UniqueID,
				"fqn":           stub.FQN,
				"resource_type": stub.ResourceType,
				"hidden":        true,
			})
			continue
		}
		depends = append(depends, map[string]any{"unique_id": d, "hidden": true})
	}
	return s.ok(map[string]any{
		"unique_id":   e.UniqueID,
		"name":        e.Name,
		"label":       e.Label,
		"type":        e.Type,
		"url":         e.URL,
		"description": e.Description,
		"tags":        e.Tags,
		"owner": map[string]string{
			"name":  e.OwnerName,
			"email": e.OwnerEmail,
		},
		"depends_on": depends,
	})
}

func (s *Service) mapErr(err error) Envelope {
	switch {
	case errors.Is(err, index.ErrNotFound):
		return s.fail("not_found", "not found")
	case errors.Is(err, index.ErrForbidden):
		return s.fail("forbidden", "forbidden")
	default:
		return s.fail("internal", err.Error())
	}
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
