package extension

import (
	"crypto/tls"
	"net/http"
)

type transport struct {
	http.RoundTripper
	Header map[string]string
}

func newTransport(tlsConfig *tls.Config, header map[string]string) http.RoundTripper {
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.TLSClientConfig = tlsConfig
	if len(header) == 0 {
		return baseTransport
	}
	return &transport{
		baseTransport, header,
	}
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	for k, v := range t.Header {
		req2.Header.Set(k, v)
	}
	return t.RoundTripper.RoundTrip(req2)
}
