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

type requestTTLHandler struct {
	verbose         bool
	cacheableStatus []int
	ttl             int
	readOnly        bool
	querier         RequestQuerier
}

func (h *requestTTLHandler) Handle(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	if r.Method != http.MethodGet {
		return r, nil
	}

	url := ctx.Req.URL.String()
	resp, err := h.querier.FindByURL(r.Context(), url)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("database query", "error", err.Error())
		}
		// tell the responseHandler to save the new response data
		ctx.UserData = userData{
			requestTime: time.Now(),
			databaseID:  -1,
		}
		return r, nil
	}
	if !slices.Contains(h.cacheableStatus, resp.Status) {
		return r, nil
	}

	if !h.readOnly && h.ttl > 0 && int(time.Since(resp.ResponseTime).Seconds()) > h.ttl {
		// data is too old, tell the responseHandler to save the new data
		ctx.UserData = userData{
			requestTime: time.Now(),
			databaseID:  resp.DatabaseID,
			tableName:   resp.TableName,
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
