package extension

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/walterwanderley/sqlite"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/litesql/httpcache/config"
	"github.com/litesql/httpcache/db"
)

type CacheModule struct {
}

func (m *CacheModule) Connect(conn *sqlite.Conn, args []string, declare func(string) error) (sqlite.VirtualTable, error) {
	tableName := args[2]
	if tableName == "" {
		tableName = config.DefaultVirtualTableName
	}
	var (
		responseTableName string
		timeout           time.Duration
		insecure          bool
		statusCodes       = config.DefaultStatusCodes()
		header            = make(map[string]string)
		credentials       clientcredentials.Config
		certFilePath      string
		certKeyFilePath   string
		caFilePath        string

		err error
	)
	if len(args) > 3 {
		for _, opt := range args[3:] {
			k, v, ok := strings.Cut(opt, "=")
			if !ok {
				return nil, fmt.Errorf("invalid option: %q", opt)
			}
			k = strings.TrimSpace(k)
			v = sanitizeOptionValue(v)

			switch strings.ToLower(k) {
			case config.Timeout:
				i, err := strconv.Atoi(v)
				if err != nil {
					return nil, fmt.Errorf("invalid %q option: %v", k, err)
				}
				timeout = time.Duration(i) * time.Millisecond
			case config.Insecure:
				insecure, err = strconv.ParseBool(v)
				if err != nil {
					return nil, fmt.Errorf("invalid %q option: %v", k, err)
				}
			case config.StatusCode:
				statusCodesParam := make([]int, 0)
				for _, statusCodeStr := range strings.Split(v, ",") {
					statusCodeStr = strings.TrimSpace(statusCodeStr)
					if len(statusCodeStr) == 0 {
						continue
					}
					statusCode, err := strconv.Atoi(statusCodeStr)
					if err != nil {
						return nil, fmt.Errorf("invalid %q option, use a comma-separated list of integers", k)
					}
					statusCodesParam = append(statusCodesParam, statusCode)
				}
				if len(statusCodesParam) > 0 {
					statusCodes = statusCodesParam
				}
			case config.ResponseTableName:
				if db.TableNameValid(v) {
					responseTableName = v
				} else {
					return nil, fmt.Errorf("invalid %q option", k)
				}
			case config.Oauth2ClientID:
				credentials.ClientID = v
			case config.Oauth2ClientSecret:
				credentials.ClientSecret = v
			case config.Oauth2TokenURL:
				credentials.TokenURL = v
			case config.CertFile:
				certFilePath = v
			case config.CertKeyFile:
				certKeyFilePath = v
			case config.CertCAFile:
				caFilePath = v
			default:
				header[k] = v
			}
		}
	}
	if responseTableName == "" {
		responseTableName = config.DefaultResponseTableName
	}

	if strings.EqualFold(tableName, responseTableName) {
		return nil, fmt.Errorf("use different names on virtual table and response table")
	}

	err = conn.Exec(db.CreateResponseTableQuery(responseTableName), nil)
	if err != nil {
		return nil, err
	}

	tlsConfig := tls.Config{
		InsecureSkipVerify: insecure,
	}

	if certFilePath != "" && certKeyFilePath != "" {
		clientCert, err := tls.LoadX509KeyPair(certFilePath, certKeyFilePath)
		if err != nil {
			return nil, fmt.Errorf("error loading client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}

	if caFilePath != "" {
		caCertPEM, err := os.ReadFile(caFilePath)
		if err != nil {
			return nil, fmt.Errorf("error loading CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCertPEM) {
			return nil, fmt.Errorf("error appending CA certificate to pool")
		}
		tlsConfig.RootCAs = caCertPool
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: newTransport(&tlsConfig, header),
	}

	if credentials.TokenURL != "" && credentials.ClientID != "" {
		ctx := context.WithValue(context.Background(), oauth2.HTTPClient, client)
		client = credentials.Client(ctx)
	}

	vtab, err := NewRequestVirtualTable(tableName, client, statusCodes, responseTableName, conn)
	if err != nil {
		return nil, err
	}
	return vtab, declare("CREATE TABLE x(url TEXT PRIMARY KEY)")
}

func sanitizeOptionValue(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "'")
	v = strings.TrimSuffix(v, "'")
	v = strings.TrimPrefix(v, "\"")
	v = strings.TrimSuffix(v, "\"")
	return os.ExpandEnv(v)
}
