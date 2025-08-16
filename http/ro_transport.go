package http

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/litesql/httpcache/db"
)

type readOnlyTransportQuerier interface {
	io.Closer
	FindByURL(ctx context.Context, url string) (*db.Response, error)
}

type readOnlyTransport struct {
	base            http.RoundTripper
	querier         readOnlyTransportQuerier
	cacheableStatus []int
	ttl             time.Duration
}

func newReadOnlyTransport(base http.RoundTripper, sqlDB *sql.DB, cacheableStatus []int, ttl time.Duration, cleanupInterval time.Duration, tableNames ...string) (*readOnlyTransport, error) {
	if base == nil {
		base = http.DefaultTransport.(*http.Transport).Clone()
	}
	querier, err := db.NewRepository(sqlDB, ttl, cleanupInterval, tableNames...)
	if err != nil {
		return nil, fmt.Errorf("creating repository: %w", err)
	}
	return &readOnlyTransport{
		base:            base,
		querier:         querier,
		cacheableStatus: cacheableStatus,
		ttl:             ttl,
	}, nil
}

func (t *readOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return t.base.RoundTrip(req)
	}

	url := req.URL.String()
	resp, err := t.querier.FindByURL(req.Context(), url)
	if err != nil || (t.ttl > 0 && time.Since(resp.ResponseTime) > t.ttl) {
		return t.base.RoundTrip(req)
	}
	if len(t.cacheableStatus) > 0 && !slices.Contains(t.cacheableStatus, resp.Status) {
		return t.base.RoundTrip(req)
	}
	return &http.Response{
		StatusCode: resp.Status,
		Body:       resp.Body,
		Header:     http.Header(resp.Header),
	}, nil

}

func (t *readOnlyTransport) Close() error {
	return t.querier.Close()
}
