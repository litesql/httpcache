package http

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"time"

	"github.com/litesql/httpcache/http/internal"
)

var HTTPClient internal.ContextKey

type transport interface {
	http.RoundTripper
	io.Closer
}

type Config struct {
	DB              *sql.DB
	Tables          []string
	ReadOnly        bool
	CacheableStatus []int
	TTL             time.Duration
	RFC9111         bool
	SharedCache     bool
	CleanupInterval time.Duration
}

func (c Config) Client(ctx context.Context) (*http.Client, error) {
	cc := internal.ContextClient(ctx)
	t, err := newTransport(cc.Transport, c)
	if err != nil {
		return nil, err
	}

	go func() {
		<-ctx.Done()
		t.Close()
	}()

	return &http.Client{
		Transport:     t,
		CheckRedirect: cc.CheckRedirect,
		Jar:           cc.Jar,
		Timeout:       cc.Timeout,
	}, nil
}
