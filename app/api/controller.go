package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/barloc/dbt-gate/app/auth"
	"github.com/barloc/dbt-gate/app/gate"
)

// Controller serves /v1/* and Streamable HTTP /mcp.
type Controller struct {
	gate        *gate.Service
	auth        auth.IdentitySource
	log         *slog.Logger
	maxJSONBody int64
	mcpHTTP     http.Handler
}

// NewController builds a Controller.
func NewController(g *gate.Service, src auth.IdentitySource, log *slog.Logger) *Controller {
	if log == nil {
		log = slog.Default()
	}
	c := &Controller{
		gate:        g,
		auth:        src,
		log:         log.With(slog.String("component", "api")),
		maxJSONBody: 1 << 20,
	}
	c.mcpHTTP = c.buildMCPHandler()
	return c
}

// Register mounts routes on the API mux.
func (c *Controller) Register(r *mux.Router) {
	r.HandleFunc("/v1/search", c.handleSearch).Methods(http.MethodPost)
	r.HandleFunc("/v1/get_model", c.handleGetModel).Methods(http.MethodPost)
	r.HandleFunc("/v1/lineage", c.handleLineage).Methods(http.MethodPost)
	r.HandleFunc("/v1/get_exposure", c.handleGetExposure).Methods(http.MethodPost)
	r.PathPrefix("/mcp").Handler(c.mcpHTTP)
}

// Handler mounts routes with the auth middleware chain.
func (c *Controller) Handler() http.Handler {
	r := mux.NewRouter()
	sr := r.PathPrefix("").Subrouter()
	sr.Use(c.authMiddleware)
	c.Register(sr)
	return r
}

// HandlerForTest is an alias of Handler for tests.
func (c *Controller) HandlerForTest() http.Handler {
	return c.Handler()
}

func (c *Controller) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := c.auth.FromRequest(r)
		if err != nil {
			c.log.Info("auth failed", slog.String("error", err.Error()))
			writeEnvelope(w, http.StatusUnauthorized, unauthorizedEnvelope())
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithIdentity(r.Context(), id)))
	})
}

func (c *Controller) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req gate.SearchRequest
	if err := decodeJSON(r, &req, c.maxJSONBody); err != nil {
		writeEnvelope(w, http.StatusBadRequest, invalidArg(err.Error()))
		return
	}
	env := c.gate.Search(r.Context(), req)
	writeEnvelope(w, statusFor(env), env)
}

func (c *Controller) handleGetModel(w http.ResponseWriter, r *http.Request) {
	var req gate.GetModelRequest
	if err := decodeJSON(r, &req, c.maxJSONBody); err != nil {
		writeEnvelope(w, http.StatusBadRequest, invalidArg(err.Error()))
		return
	}
	env := c.gate.GetModel(r.Context(), req)
	writeEnvelope(w, statusFor(env), env)
}

func (c *Controller) handleLineage(w http.ResponseWriter, r *http.Request) {
	var req gate.LineageRequest
	if err := decodeJSON(r, &req, c.maxJSONBody); err != nil {
		writeEnvelope(w, http.StatusBadRequest, invalidArg(err.Error()))
		return
	}
	env := c.gate.Lineage(r.Context(), req)
	writeEnvelope(w, statusFor(env), env)
}

func (c *Controller) handleGetExposure(w http.ResponseWriter, r *http.Request) {
	var req gate.GetExposureRequest
	if err := decodeJSON(r, &req, c.maxJSONBody); err != nil {
		writeEnvelope(w, http.StatusBadRequest, invalidArg(err.Error()))
		return
	}
	env := c.gate.GetExposure(r.Context(), req)
	writeEnvelope(w, statusFor(env), env)
}

func decodeJSON(r *http.Request, dst any, maxBody int64) (err error) {
	defer func() {
		if cerr := r.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	if maxBody <= 0 {
		maxBody = 1 << 20
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()
	err = dec.Decode(dst)
	if errors.Is(err, io.EOF) {
		return errors.New("empty body")
	}
	return err
}

func writeEnvelope(w http.ResponseWriter, status int, env gate.Envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(env); err != nil {
		return
	}
}

func statusFor(env gate.Envelope) int {
	if env.OK {
		return http.StatusOK
	}
	if env.Error == nil {
		return http.StatusInternalServerError
	}
	switch env.Error.Code {
	case "invalid_argument":
		return http.StatusBadRequest
	case "unauthorized":
		return http.StatusUnauthorized
	case "forbidden":
		return http.StatusForbidden
	case "not_found":
		return http.StatusNotFound
	case "index_unavailable":
		return http.StatusServiceUnavailable
	case "internal":
		return http.StatusInternalServerError
	default:
		return http.StatusBadRequest
	}
}

func unauthorizedEnvelope() gate.Envelope {
	return gate.Envelope{
		OK:         false,
		APIVersion: 1,
		Error:      &gate.Error{Code: "unauthorized", Message: "unauthorized"},
	}
}

func invalidArg(msg string) gate.Envelope {
	return gate.Envelope{
		OK:         false,
		APIVersion: 1,
		Error:      &gate.Error{Code: "invalid_argument", Message: msg},
	}
}
