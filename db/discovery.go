package db

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
)

func ResponseTables(db *sql.DB) ([]string, error) {
	var tables sql.NullString
	err := db.QueryRow("SELECT group_concat(name, '|') FROM sqlite_schema WHERE type = 'table'").Scan(&tables)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}

	if !tables.Valid {
		return nil, sql.ErrNoRows
	}

	responseTables := make([]string, 0)
	for _, table := range strings.Split(tables.String, "|") {
		ok, err := isResponseTable(db, table)
		if err != nil {
			return nil, err
		}
		if ok {
			responseTables = append(responseTables, table)
		}
	}

	if len(responseTables) == 0 {
		return nil, sql.ErrNoRows
	}

	return responseTables, nil
}

func isResponseTable(db *sql.DB, table string) (bool, error) {
	rows, err := db.Query("SELECT lower(name) FROM PRAGMA_TABLE_INFO(?)", table)
	if err != nil {
		return false, fmt.Errorf("list columns: %w", err)
	}
	defer rows.Close()

	columns := make([]string, 0)
	var name string
	for rows.Next() {
		err := rows.Scan(&name)
		if err != nil {
			return false, fmt.Errorf("read column name: %w", err)
		}
		columns = append(columns, name)
	}

	return slices.Contains(columns, "url") && slices.Contains(columns, "status") &&
		slices.Contains(columns, "body") && slices.Contains(columns, "header") &&
		slices.Contains(columns, "request_time") && slices.Contains(columns, "response_time"), nil

}
