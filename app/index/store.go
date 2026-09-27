package index

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

type (
	configFile struct {
		Product string       `yaml:"product"`
		Index   IndexConfig  `yaml:"index"`
		Search  SearchConfig `yaml:"search"`
	}

	// IndexConfig is YAML index block.
	IndexConfig struct {
		URL             string `yaml:"url"`
		File            string `yaml:"file"`
		RefreshInterval string `yaml:"refresh_interval"`
		IncludeCatalog  *bool  `yaml:"include_catalog"`
	}

	// SearchConfig is YAML search block.
	SearchConfig struct {
		MaxHits int `yaml:"max_hits"`
	}

	// Config is runtime index settings.
	Config struct {
		Product         string
		URL             string
		File            string
		RefreshInterval time.Duration
		MaxHits         int
		IncludeCatalog  bool
	}

	// Store holds the atomic index pointer and refresh loop.
	Store struct {
		log   *slog.Logger
		cfg   Config
		fetch Fetcher
		cur   atomic.Pointer[Index]
		etag  atomic.Value // string
	}
)

// ParseConfig unmarshals product/index/search from YAML.
func ParseConfig(raw []byte) (Config, error) {
	var cf configFile
	if err := yaml.Unmarshal(raw, &cf); err != nil {
		return Config{}, fmt.Errorf("index config: %w", err)
	}
	cfg := Config{
		Product:        cf.Product,
		URL:            cf.Index.URL,
		File:           cf.Index.File,
		MaxHits:        cf.Search.MaxHits,
		IncludeCatalog: true,
	}
	if cf.Index.IncludeCatalog != nil {
		cfg.IncludeCatalog = *cf.Index.IncludeCatalog
	}
	if cfg.Product == "" {
		cfg.Product = "example"
	}
	if cfg.URL == "" && cfg.File == "" {
		return Config{}, fmt.Errorf("index: set index.url or index.file")
	}
	cfg.RefreshInterval = 5 * time.Minute
	if cf.Index.RefreshInterval != "" {
		d, err := time.ParseDuration(cf.Index.RefreshInterval)
		if err != nil {
			return Config{}, fmt.Errorf("index.refresh_interval: %w", err)
		}
		cfg.RefreshInterval = d
	}
	if cfg.MaxHits <= 0 {
		cfg.MaxHits = 20
	}
	return cfg, nil
}

// NewStore constructs an empty Store (load via Start).
func NewStore(cfg Config, log *slog.Logger) *Store {
	return newStore(cfg, log)
}

// NewTestStore builds a Store for unit tests.
func NewTestStore(product string) *Store {
	if product == "" {
		product = "example"
	}
	return newStore(Config{
		Product:         product,
		MaxHits:         20,
		RefreshInterval: time.Minute,
		IncludeCatalog:  true,
	}, slog.Default())
}

func newStore(cfg Config, log *slog.Logger) *Store {
	if log == nil {
		log = slog.Default()
	}
	s := &Store{
		log: log.With(slog.String("component", "index")),
		cfg: cfg,
		fetch: Fetcher{
			URL:  cfg.URL,
			File: cfg.File,
		},
	}
	s.etag.Store("")
	s.setReady(0)
	return s
}

// Get returns the current index or nil if not ready.
func (s *Store) Get() *Index {
	return s.cur.Load()
}

// Product returns configured product.
func (s *Store) Product() string { return s.cfg.Product }

// MaxHits returns configured search cap.
func (s *Store) MaxHits() int { return s.cfg.MaxHits }

// Start kicks off initial load + periodic refresh in the background.
// /v1 returns 503 index_unavailable until the first successful load.
func (s *Store) Start(ctx context.Context) {
	go s.loop(ctx)
}

func (s *Store) loop(ctx context.Context) {
	s.refresh(ctx)
	t := time.NewTicker(s.cfg.RefreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.refresh(ctx)
		}
	}
}

func (s *Store) refresh(ctx context.Context) {
	prev, ok := s.etag.Load().(string)
	if !ok {
		prev = ""
	}
	res, err := s.fetch.HeadOrGet(prev)
	if err != nil {
		s.log.Error("index refresh failed", slog.String("error", err.Error()))
		s.observeRefreshError()
		return
	}
	if res.NotModified {
		s.log.Debug("index unchanged", slog.String("etag", prev))
		if s.Get() != nil {
			s.observeLoaded()
		}
		return
	}
	idx, err := Build(res.Body, s.cfg.Product, res.Checksum, res.ETag, s.cfg.IncludeCatalog)
	if err != nil {
		s.log.Error("index build failed", slog.String("error", err.Error()))
		s.observeRefreshError()
		return
	}
	select {
	case <-ctx.Done():
		return
	default:
	}
	s.cur.Store(idx)
	s.etag.Store(res.ETag)
	s.observeIndex(idx)
	s.log.Info("index loaded",
		slog.String("product", idx.Meta.Product),
		slog.String("checksum", idx.Meta.Checksum),
		slog.String("generated_at", idx.Meta.ManifestGeneratedAt),
		slog.Int("nodes", len(idx.ByUID)),
		slog.Int("exposures", len(idx.Exposures)),
		slog.Int("search_docs", len(idx.SearchDocs)),
	)
}

// LoadBytes builds and swaps an index from raw bytes (tests).
func (s *Store) LoadBytes(body []byte, checksum string) error {
	idx, err := Build(body, s.cfg.Product, checksum, checksum, s.cfg.IncludeCatalog)
	if err != nil {
		return err
	}
	s.cur.Store(idx)
	s.etag.Store(checksum)
	s.observeIndex(idx)
	return nil
}
