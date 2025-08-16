package db

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Repository interface {
	io.Closer
	FindByURL(ctx context.Context, url string) (*Response, error)
	Write(ctx context.Context, url string, resp *Response) error
}

type Response struct {
	Status       int
	Body         io.ReadCloser
	Header       map[string][]string
	RequestTime  time.Time
	ResponseTime time.Time
	DatabaseID   int
	TableName    string
}

func NewRepository(db *sql.DB, ttl time.Duration, cleanupInterval time.Duration, tableNames ...string) (Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	if len(tableNames) == 0 {
		var err error
		tableNames, err = ResponseTables(db)
		if err != nil {
			return nil, fmt.Errorf("discovery response tables: %w", err)
		}
	}

	if len(tableNames) == 1 {
		return newSingleRepository(db, tableNames[0], ttl, cleanupInterval)
	}

	return newConcurrentRepository(db, 0, ttl, cleanupInterval, tableNames...)
}

func CreateResponseTableQuery(tableName string) string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s(
		url TEXT PRIMARY KEY,
		status INTEGER,
		body BLOB,
		header JSONB,
		request_time DATETIME,
		response_time DATETIME
		)`, tableName)
}

var TableNameValid = regexp.MustCompilePOSIX("^[a-zA-Z_][a-zA-Z0-9_.]*$").MatchString

func CreateResponseTables(db *sql.DB, tableNames ...string) error {

	for _, tableName := range tableNames {
		if !TableNameValid(tableName) {
			return fmt.Errorf("tabe name %q is invalid", tableName)
		}
		query := CreateResponseTableQuery(tableName)
		_, err := db.Exec(query)
		if err != nil {
			return fmt.Errorf("creating table %q: %w", tableName, err)
		}
	}
	return nil
}

func HttpToResponse(resp *http.Response) (*Response, error) {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	return &Response{
		Status: resp.StatusCode,
		Header: resp.Header,
		Body:   io.NopCloser(bytes.NewReader(bodyBytes)),
	}, nil
}

func readerQuery(tableName string) string {
	return fmt.Sprintf("SELECT status, body, header, request_time, response_time FROM %s WHERE url = ?", tableName)
}

func rowToResponse(row *sql.Row) (*Response, error) {
	if err := row.Err(); err != nil {
		return nil, fmt.Errorf("response row: %w", err)
	}
	var (
		status       int
		body         string
		header       string
		requestTime  time.Time
		responseTime time.Time
	)
	err := row.Scan(&status, &body, &header, &requestTime, &responseTime)
	if err != nil {
		return nil, fmt.Errorf("scan response row: %w", err)
	}
	var headerMap map[string][]string
	json.Unmarshal([]byte(header), &headerMap)

	return &Response{
		Status:       status,
		Body:         io.NopCloser(strings.NewReader(body)),
		Header:       headerMap,
		RequestTime:  requestTime,
		ResponseTime: responseTime,
	}, nil
}

func WriterQuery(tableName string) string {
	return fmt.Sprintf(`INSERT INTO %s(url, status, body, header, request_time, response_time) 
	VALUES(?, ?, ?, ?, ?, ?)
	ON CONFLICT(url) DO UPDATE SET 
	status = ?,
	body = ?,
	header = ?,
	request_time = ?,
	response_time = ?`, tableName)
}

func execWriter(ctx context.Context, stmt *sql.Stmt, url string, resp *Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading body: %w", err)
	}
	bodyStr := string(body)

	var headerBuf bytes.Buffer
	json.NewEncoder(&headerBuf).Encode(resp.Header)
	header := headerBuf.String()

	formattedRequetTime := resp.RequestTime.Format(time.RFC3339Nano)
	formattedResponseTime := resp.ResponseTime.Format(time.RFC3339Nano)

	_, err = stmt.ExecContext(ctx,
		url, resp.Status, bodyStr, header, formattedRequetTime, formattedResponseTime,
		// On Conflict
		resp.Status, bodyStr, header, formattedRequetTime, formattedResponseTime)

	if err != nil {
		return fmt.Errorf("store response: %w", err)
	}
	return nil
}

func cleanupByTTLQuery(tableName string) string {
	return fmt.Sprintf("DELETE FROM %s WHERE rowid IN (SELECT rowid FROM %s WHERE unixepoch() - unixepoch(response_time) > ? ORDER BY rowid LIMIT 1000)", tableName, tableName)
}
