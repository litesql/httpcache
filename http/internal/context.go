package internal

import (
	"context"
	"net/http"
)

var HTTPClient ContextKey

type ContextKey struct{}

func ContextClient(ctx context.Context) *http.Client {
	if ctx != nil {
		if hc, ok := ctx.Value(HTTPClient).(*http.Client); ok {
			return hc
		}
	}
	return http.DefaultClient
}
