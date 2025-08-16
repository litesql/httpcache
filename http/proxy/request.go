package proxy

import (
	"context"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/litesql/httpcache/db"
)

type RequestConfig struct {
	Querier         RequestQuerier
	CacheableStatus []int
	TTL             int
	RFC9111         bool
	SharedCache     bool
	ReadOnly        bool
	Verbose         bool
}

type RequestQuerier interface {
	FindByURL(ctx context.Context, url string) (*db.Response, error)
}

func NewRequestHandler(config RequestConfig) goproxy.ReqHandler {
	if config.RFC9111 {
		return &requestRFC9111Handler{
			shared:          config.SharedCache,
			cacheableStatus: config.CacheableStatus,
			verbose:         config.Verbose,
			readOnly:        config.ReadOnly,
			querier:         config.Querier,
		}
	}
	return &requestTTLHandler{
		verbose:         config.Verbose,
		cacheableStatus: config.CacheableStatus,
		ttl:             config.TTL,
		readOnly:        config.ReadOnly,
		querier:         config.Querier,
	}
}

type userData struct {
	requestTime time.Time
	databaseID  int
	tableName   string
}
