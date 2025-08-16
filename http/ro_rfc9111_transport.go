package http

import (
	"database/sql"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/litesql/httpcache/config"
	"github.com/litesql/httpcache/db"
)

type readOnlyRFC9111Transport struct {
	base            http.RoundTripper
	querier         readOnlyTransportQuerier
	cacheableStatus []int
	ttl             time.Duration
	sharedCache     bool
}

func newReadOnlyRFC9111Transport(base http.RoundTripper, sqlDB *sql.DB, cacheableStatus []int, sharedCache bool, ttl time.Duration, cleanupInterval time.Duration, tableNames ...string) (*readOnlyRFC9111Transport, error) {
	if base == nil {
		base = http.DefaultTransport.(*http.Transport).Clone()
	}
	querier, err := db.NewRepository(sqlDB, ttl, cleanupInterval, tableNames...)
	if err != nil {
		return nil, fmt.Errorf("creating repository: %w", err)
	}
	if len(cacheableStatus) == 0 {
		cacheableStatus = config.DefaultStatusCodes()
	}
	return &readOnlyRFC9111Transport{
		base:            base,
		cacheableStatus: cacheableStatus,
		querier:         querier,
		ttl:             ttl,
		sharedCache:     sharedCache,
	}, nil
}

func (t *readOnlyRFC9111Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return t.base.RoundTrip(req)
	}

	ttlSeconds := int(t.ttl.Seconds())
	cc := ParseCacheControl(req.Header, nil, nil, t.sharedCache, ttlSeconds)
	if !cc.Cacheable() {
		return t.base.RoundTrip(req)
	}
	if t.sharedCache && req.Header.Get("Authorization") != "" {
		return t.base.RoundTrip(req)
	}

	url := req.URL.String()
	resp, err := t.querier.FindByURL(req.Context(), url)
	if err != nil {
		return t.base.RoundTrip(req)
	}

	if !slices.Contains(t.cacheableStatus, resp.Status) {
		return t.base.RoundTrip(req)
	}

	respCC := ParseCacheControl(http.Header(resp.Header), &resp.RequestTime, &resp.ResponseTime, t.sharedCache, ttlSeconds)
	if respCC.Expired() {
		return t.base.RoundTrip(req)
	}

	return &http.Response{
		StatusCode: resp.Status,
		Body:       resp.Body,
		Header:     http.Header(resp.Header),
	}, nil

}

func (t *readOnlyRFC9111Transport) Close() error {
	return t.querier.Close()
}
