package proxy

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/litesql/httpcache/db"
)

type responseTTLHandler struct {
	writer  ResponseWriter
	verbose bool
}

func (h *responseTTLHandler) Handle(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	ud, ok := ctx.UserData.(userData)
	if ok {
		if h.verbose {
			slog.Info("recording response", "url", ctx.Req.URL.String(), "status", resp.StatusCode)
		}
		responseDB, err := db.HttpToResponse(resp)
		if err != nil {
			slog.Error("adapter response body", "error", err)
		} else {
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
	}
	return resp
}
