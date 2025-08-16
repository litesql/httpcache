package extension

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/walterwanderley/sqlite"

	"github.com/litesql/httpcache/db"
)

type RequestVirtualTable struct {
	client            *http.Client
	statusCodes       []int
	tableName         string
	responseTableName string
	conn              *sqlite.Conn
	stmt              *sqlite.Stmt
	mu                sync.Mutex
}

func NewRequestVirtualTable(virtualTableName string, client *http.Client, statusCodes []int, responseTableName string, conn *sqlite.Conn) (*RequestVirtualTable, error) {
	stmt, _, err := conn.Prepare(db.WriterQuery(responseTableName))
	if err != nil {
		return nil, err
	}

	return &RequestVirtualTable{
		client:            client,
		statusCodes:       statusCodes,
		tableName:         virtualTableName,
		responseTableName: responseTableName,
		conn:              conn,
		stmt:              stmt,
	}, nil
}

func (vt *RequestVirtualTable) BestIndex(in *sqlite.IndexInfoInput) (*sqlite.IndexInfoOutput, error) {
	return &sqlite.IndexInfoOutput{}, nil
}

func (vt *RequestVirtualTable) Open() (sqlite.VirtualCursor, error) {
	return nil, fmt.Errorf("SELECT operations on %q is not supported, exec SELECT on %q table", vt.tableName, vt.responseTableName)
}

func (vt *RequestVirtualTable) Disconnect() error {
	return vt.stmt.Finalize()
}

func (vt *RequestVirtualTable) Destroy() error {
	return nil
}

func (vt *RequestVirtualTable) Insert(values ...sqlite.Value) (int64, error) {
	url := values[0].Text()
	requestTime := time.Now().Format(time.RFC3339Nano)
	resp, err := vt.client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if len(vt.statusCodes) > 0 && !slices.Contains(vt.statusCodes, resp.StatusCode) {
		return 0, nil
	}

	if resp.Header.Get("Date") == "" {
		resp.Header.Set("Date", time.Now().Format(time.RFC1123))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	body := string(bodyBytes)
	status := int64(resp.StatusCode)

	var headerBuf bytes.Buffer
	json.NewEncoder(&headerBuf).Encode(resp.Header)
	header := headerBuf.String()
	responseTime := time.Now().Format(time.RFC3339Nano)
	vt.mu.Lock()
	defer vt.mu.Unlock()
	err = vt.stmt.Reset()
	if err != nil {
		return 0, err
	}
	vt.stmt.BindText(1, url)
	vt.stmt.BindInt64(2, status)
	vt.stmt.BindText(3, body)
	vt.stmt.BindText(4, header)
	vt.stmt.BindText(5, requestTime)
	vt.stmt.BindText(6, responseTime)
	//ON CONFLICT
	vt.stmt.BindInt64(7, status)
	vt.stmt.BindText(8, body)
	vt.stmt.BindText(9, header)
	vt.stmt.BindText(10, requestTime)
	vt.stmt.BindText(11, responseTime)
	_, err = vt.stmt.Step()
	if err != nil {
		return 0, err
	}
	return 0, nil
}

func (vt *RequestVirtualTable) Update(_ sqlite.Value, _ ...sqlite.Value) error {
	return fmt.Errorf("UPDATE operations on %q is not supported", vt.tableName)
}

func (vt *RequestVirtualTable) Replace(old sqlite.Value, new sqlite.Value, _ ...sqlite.Value) error {
	return fmt.Errorf("UPDATE operations on %q is not supported", vt.tableName)
}

func (vt *RequestVirtualTable) Delete(_ sqlite.Value) error {
	return fmt.Errorf("DELETE operations on %q is not supported", vt.tableName)
}
