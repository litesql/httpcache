package config

import "slices"

const (
	Timeout            = "timeout"              // timeout in milliseconds
	Insecure           = "insecure"             // insecure skip TLS validation
	StatusCode         = "status_code"          // Comma-separated list of HTTP status to persist
	ResponseTableName  = "response_table"       // table name of the http responses
	Oauth2ClientID     = "oauth2_client_id"     // oauth2 client credentials flow: cient_id
	Oauth2ClientSecret = "oauth2_client_secret" // oauth2 client credentials flow: cient_secret
	Oauth2TokenURL     = "oauth2_token_url"     // oauth2 client credentials flow: token URL
	CertFile           = "cert_file"            // mutual TLS: path to certificate file
	CertKeyFile        = "cert_key_file"        // mutual TLS: path to .pem certificate key file
	CertCAFile         = "ca_file"              // path to CA certificate file

	DefaultResponseTableName = "http_response"
	DefaultVirtualTableName  = "http_request"
)

var defaultStatusCodes = []int{200, 203, 204, 206, 300, 301, 308, 404, 405, 410, 414, 501}

func DefaultStatusCodes() []int {
	return slices.Clone(defaultStatusCodes)
}
