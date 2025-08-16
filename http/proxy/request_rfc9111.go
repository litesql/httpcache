package proxy

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/elazarl/goproxy"

	cachehttp "github.com/litesql/httpcache/http"
)

type requestRFC9111Handler struct {
	shared          bool
	cacheableStatus []int
	ttlFallback     int
	verbose         bool
	readOnly        bool
	querier         RequestQuerier
}

func (h *requestRFC9111Handler) Handle(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	if r.Method != http.MethodGet {
		return r, nil
	}

	cc := cachehttp.ParseCacheControl(r.Header, nil, nil, h.shared, h.ttlFallback)
	if !cc.Cacheable() {
		return r, nil
	}
	if h.shared && r.Header.Get("Authorization") != "" {
		return r, nil
	}

	now := time.Now()
	url := ctx.Req.URL.String()
	resp, err := h.querier.FindByURL(r.Context(), url)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("database query", "error", err.Error())
		}

		if !h.readOnly {
			// tell the responseHandler to save the new response data
			ctx.UserData = userData{
				requestTime: now,
				databaseID:  -1,
			}
		}
		return r, nil
	}

	if !slices.Contains(h.cacheableStatus, resp.Status) {
		return r, nil
	}

	respCC := cachehttp.ParseCacheControl(http.Header(resp.Header), &resp.RequestTime, &resp.ResponseTime, h.shared, h.ttlFallback)

	if respCC.Expired() {
		if !h.readOnly {
			// data is too old, tell the responseHandler to save the new data
			ctx.UserData = userData{
				requestTime: now,
				databaseID:  resp.DatabaseID,
				tableName:   resp.TableName,
			}
		}
		return r, nil
	}
	if h.verbose {
		slog.Info("serving from database", "url", url, "status", resp.Status, "request_time", resp.RequestTime.Format(time.RFC3339), "response_time", resp.ResponseTime.Format(time.RFC3339))
	}

	header := http.Header(resp.Header)
	if header.Get("Date") == "" {
		header.Set("Date", time.Now().Format(time.RFC1123))
	}
	if age := cachehttp.Age(header, resp.RequestTime, resp.ResponseTime); age != nil {
		header.Set("Age", fmt.Sprint(*age))
	}

	return r, &http.Response{
		StatusCode: resp.Status,
		Body:       resp.Body,
		Header:     header,
	}
}
