package index

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Fetcher loads manifest bytes from URL or local file.
type Fetcher struct {
	URL    string
	File   string
	Client *http.Client
}

// FetchResult is a downloaded (or local) manifest payload.
type FetchResult struct {
	Body        []byte
	ETag        string
	Checksum    string // Artifactory SHA1 when present
	NotModified bool
}

// HeadOrGet checks ETag via HEAD (URL) or mtime/size (file), then GETs if changed.
func (f *Fetcher) HeadOrGet(prevETag string) (FetchResult, error) {
	if f.File != "" {
		return f.fetchFile(prevETag)
	}
	if f.URL == "" {
		return FetchResult{}, fmt.Errorf("index: neither url nor file configured")
	}
	return f.fetchURL(prevETag)
}

func (f *Fetcher) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 120 * time.Second}
}

func (f *Fetcher) fetchURL(prevETag string) (res FetchResult, err error) {
	c := f.client()
	if prevETag != "" {
		var req *http.Request
		req, err = http.NewRequest(http.MethodHead, f.URL, nil)
		if err != nil {
			return FetchResult{}, err
		}
		req.Header.Set("If-None-Match", prevETag)
		var resp *http.Response
		resp, err = c.Do(req)
		if err != nil {
			return FetchResult{}, fmt.Errorf("index: HEAD: %w", err)
		}
		if cerr := resp.Body.Close(); cerr != nil {
			return FetchResult{}, fmt.Errorf("index: HEAD close: %w", cerr)
		}
		if resp.StatusCode == http.StatusNotModified {
			return FetchResult{NotModified: true, ETag: prevETag, Checksum: prevETag}, nil
		}
		if resp.StatusCode == http.StatusOK {
			etag := normalizeETag(resp.Header.Get("ETag"))
			if etag == "" {
				etag = normalizeETag(resp.Header.Get("X-Checksum-Sha1"))
			}
			if etag != "" && etag == prevETag {
				return FetchResult{NotModified: true, ETag: etag, Checksum: etag}, nil
			}
		}
		// Non-OK HEAD (or CDN quirks): fall through to conditional GET.
	}

	req, err := http.NewRequest(http.MethodGet, f.URL, nil)
	if err != nil {
		return FetchResult{}, err
	}
	if prevETag != "" {
		req.Header.Set("If-None-Match", prevETag)
	}
	resp, err := c.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("index: GET: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("index: GET close: %w", cerr)
		}
	}()

	if resp.StatusCode == http.StatusNotModified {
		return FetchResult{NotModified: true, ETag: prevETag, Checksum: prevETag}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return FetchResult{}, fmt.Errorf("index: GET status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return FetchResult{}, fmt.Errorf("index: read body: %w", err)
	}
	etag := normalizeETag(resp.Header.Get("ETag"))
	sha1 := normalizeETag(resp.Header.Get("X-Checksum-Sha1"))
	if etag == "" {
		etag = sha1
	}
	checksum := sha1
	if checksum == "" {
		checksum = etag
	}
	return FetchResult{Body: body, ETag: etag, Checksum: checksum}, nil
}

func (f *Fetcher) fetchFile(prevETag string) (FetchResult, error) {
	st, err := os.Stat(f.File)
	if err != nil {
		return FetchResult{}, fmt.Errorf("index: stat file: %w", err)
	}
	tag := fmt.Sprintf("%d-%d", st.ModTime().UnixNano(), st.Size())
	if prevETag != "" && prevETag == tag {
		return FetchResult{NotModified: true, ETag: tag, Checksum: tag}, nil
	}
	body, err := os.ReadFile(f.File)
	if err != nil {
		return FetchResult{}, fmt.Errorf("index: read file: %w", err)
	}
	return FetchResult{Body: body, ETag: tag, Checksum: tag}, nil
}

func normalizeETag(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "W/")
	s = strings.Trim(s, `"`)
	return s
}
