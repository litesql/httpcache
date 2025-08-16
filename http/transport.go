package http

import "net/http"

func newTransport(base http.RoundTripper, config Config) (transport, error) {
	if config.ReadOnly {
		if config.RFC9111 {
			return newReadOnlyRFC9111Transport(base, config.DB, config.CacheableStatus, config.SharedCache, config.TTL, config.CleanupInterval, config.Tables...)
		}
		return newReadOnlyTransport(base, config.DB, config.CacheableStatus, config.TTL, config.CleanupInterval, config.Tables...)
	}
	if config.RFC9111 {
		return newReadWriteRFC9111Transport(base, config.DB, config.CacheableStatus, config.SharedCache, config.TTL, config.CleanupInterval, config.Tables...)
	}
	return newReadWriteTransport(base, config.DB, config.CacheableStatus, config.TTL, config.CleanupInterval, config.Tables...)
}
