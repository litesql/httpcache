package http

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/litesql/httpcache/db"
)

type readWriteTransportQuerier interface {
	readOnlyTransportQuerier
	Write(ctx context.Context, url string, resp *db.Response) error
}

type readWriteTransport struct {
	base            http.RoundTripper
	querier         readWriteTransportQuerier
	cacheableStatus []int
	ttl             time.Duration
}

func newReadWriteTransport(base http.RoundTripper, sqlDB *sql.DB, cacheableStatus []int, ttl time.Duration, cleanupInterval time.Duration, tableNames ...string) (*readWriteTransport, error) {
	if base == nil {
		base = http.DefaultTransport.(*http.Transport).Clone()
	}
	querier, err := db.NewRepository(sqlDB, ttl, cleanupInterval, tableNames...)
	if err != nil {
		return nil, fmt.Errorf("creating repository: %w", err)
	}
	return &readWriteTransport{
		base:            base,
		querier:         querier,
		cacheableStatus: cacheableStatus,
		ttl:             ttl,
	}, nil
}

func (t *readWriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return t.base.RoundTrip(req)
	}

	var (
		resp *http.Response
		err  error
	)
	url := req.URL.String()
	respDB, err := t.querier.FindByURL(req.Context(), url)
	if err != nil {
		resp, err = t.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
	}

	if respDB != nil && !((t.ttl > 0 && time.Since(respDB.ResponseTime) > t.ttl) ||
		(len(t.cacheableStatus) > 0 && !slices.Contains(t.cacheableStatus, respDB.Status))) {
		return &http.Response{
			StatusCode: respDB.Status,
			Body:       respDB.Body,
			Header:     http.Header(respDB.Header),
		}, nil
	}
	if resp == nil {
		resp, err = t.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
	}
	newRespDB, err := db.HttpToResponse(resp)
	if err == nil {
		if respDB != nil {
			newRespDB.TableName = respDB.TableName
		}
		t.querier.Write(context.Background(), url, newRespDB)
	}

	return resp, err
}

func (t *readWriteTransport) Close() error {
	return t.querier.Close()
}
