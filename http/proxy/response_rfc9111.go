package proxy

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/litesql/httpcache/db"
	cachehttp "github.com/litesql/httpcache/http"
)

type responseRFC9111Handler struct {
	shared      bool
	ttlFallback int
	writer      ResponseWriter
	verbose     bool
}

func (h *responseRFC9111Handler) Handle(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	requestTime := time.Now()
	ud, ok := ctx.UserData.(userData)
	if ok {
		requestTime = ud.requestTime
	}

	responseTime := time.Now()
	cc := cachehttp.ParseCacheControl(resp.Header, &requestTime, &responseTime, h.shared, h.ttlFallback)
	if !cc.Cacheable() || !ok {
		return resp
	}

	responseDB, err := db.HttpToResponse(resp)
	if err != nil {
		slog.Error("adapter response body", "error", err)
	} else {
		if !cc.Expired() {
			return resp
		}
		if h.verbose {
			slog.Info("recording response", "url", ctx.Req.URL.String(), "status", resp.StatusCode)
		}
		go func() {
			responseDB.RequestTime = ud.requestTime
			responseDB.ResponseTime = time.Now()
			responseDB.DatabaseID = ud.databaseID
			responseDB.TableName = ud.tableName
			err := h.writer.Write(context.Background(), ctx.Req.URL.String(), responseDB)
			if err != nil {
				slog.Error("recording response", "error", err, "url", ctx.Req.URL.String(), "status", resp.StatusCode)
			}
		}()
	}

	return resp
}
