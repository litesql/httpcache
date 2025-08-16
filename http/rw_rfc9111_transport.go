package http

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/litesql/httpcache/config"
	"github.com/litesql/httpcache/db"
)

type readWriteRFC9111Transport struct {
	base            http.RoundTripper
	querier         readWriteTransportQuerier
	cacheableStatus []int
	ttl             time.Duration
	sharedCache     bool
}

func newReadWriteRFC9111Transport(base http.RoundTripper, sqlDB *sql.DB, cacheableStatus []int, sharedCache bool, ttl time.Duration, cleanupInterval time.Duration, tableNames ...string) (*readWriteRFC9111Transport, error) {
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
	return &readWriteRFC9111Transport{
		base:            base,
		querier:         querier,
		cacheableStatus: cacheableStatus,
		ttl:             ttl,
		sharedCache:     sharedCache,
	}, nil
}

func (t *readWriteRFC9111Transport) RoundTrip(req *http.Request) (*http.Response, error) {
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

	var (
		requestTime  time.Time
		responseTime time.Time
		resp         *http.Response
		err          error
	)
	url := req.URL.String()
	respDB, err := t.querier.FindByURL(req.Context(), url)
	if err != nil {
		requestTime = time.Now()
		resp, err = t.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		responseTime = time.Now()
	}

	if respDB != nil {
		respCC := ParseCacheControl(http.Header(respDB.Header), &respDB.RequestTime, &respDB.ResponseTime, t.sharedCache, ttlSeconds)
		if !respCC.Expired() {
			return &http.Response{
				StatusCode: respDB.Status,
				Body:       respDB.Body,
				Header:     http.Header(respDB.Header),
			}, nil
		}
	}

	if resp == nil {
		requestTime = time.Now()
		resp, err = t.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		responseTime = time.Now()
	}
	respCC := ParseCacheControl(http.Header(resp.Header), &requestTime, &responseTime, t.sharedCache, ttlSeconds)
	if slices.Contains(t.cacheableStatus, resp.StatusCode) && respCC.Cacheable() {
		newRespDB, err := db.HttpToResponse(resp)
		if err == nil {
			if respDB != nil {
				newRespDB.TableName = respDB.TableName
			}
			t.querier.Write(context.Background(), url, newRespDB)
		}

	}

	return resp, err
}

func (t *readWriteRFC9111Transport) Close() error {
	return t.querier.Close()
}
