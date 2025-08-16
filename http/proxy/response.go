package proxy

import (
	"context"

	"github.com/elazarl/goproxy"
	"github.com/litesql/httpcache/db"
)

type ResponseConfig struct {
	Writer      ResponseWriter
	RFC9111     bool
	TTL         int
	Verbose     bool
	SharedCache bool
}

type ResponseWriter interface {
	Write(ctx context.Context, url string, resp *db.Response) error
}

func NewResponseHandler(config ResponseConfig) goproxy.RespHandler {
	if config.RFC9111 {
		return &responseRFC9111Handler{
			shared:      config.SharedCache,
			ttlFallback: config.TTL,
			writer:      config.Writer,
			verbose:     config.Verbose,
		}
	}
	return &responseTTLHandler{
		writer:  config.Writer,
		verbose: config.Verbose,
	}
}
