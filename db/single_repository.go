package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type singleRepository struct {
	readStmt    *sql.Stmt
	writeStmt   *sql.Stmt
	cleanupStmt *sql.Stmt
	mu          sync.Mutex

	cleanupCancelation func()
	ttl                int64 // time to live in miliseconds
}

func newSingleRepository(db *sql.DB, tableName string, ttl time.Duration, cleanupInterval time.Duration) (*singleRepository, error) {
	readStmt, err := db.Prepare(readerQuery(tableName))
	if err != nil {
		return nil, fmt.Errorf("prepare reader query for %q: %w", tableName, err)
	}
	writeStmt, err := db.Prepare(WriterQuery(tableName))
	if err != nil {
		return nil, fmt.Errorf("prepare writer query for %q: %w", tableName, err)
	}
	cleanupStmt, err := db.Prepare(cleanupByTTLQuery(tableName))
	if err != nil {
		return nil, fmt.Errorf("prepare cleanup query for %q: %w", tableName, err)
	}
	repository := singleRepository{
		readStmt:    readStmt,
		writeStmt:   writeStmt,
		cleanupStmt: cleanupStmt,
		ttl:         int64(ttl.Seconds()),
	}
	if cleanupInterval > 0 && ttl > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		repository.cleanupCancelation = cancel
		slog.Info("Repository cleanup started", "ttl", ttl, "interval", cleanupInterval)
		go func() {
			ticker := time.NewTicker(cleanupInterval)
			for {
				select {
				case <-ticker.C:
					repository.cleanup()
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	return &repository, nil
}

func (r *singleRepository) FindByURL(ctx context.Context, url string) (*Response, error) {
	response, err := rowToResponse(r.readStmt.QueryRowContext(ctx, url))
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (r *singleRepository) Write(ctx context.Context, url string, resp *Response) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return execWriter(ctx, r.writeStmt, url, resp)
}

func (r *singleRepository) Close() error {
	if r.cleanupCancelation != nil {
		r.cleanupCancelation()
	}
	return errors.Join(r.readStmt.Close(), r.writeStmt.Close(), r.cleanupStmt.Close())
}

func (r *singleRepository) cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for {
		res, err := r.cleanupStmt.Exec(r.ttl)
		if err != nil {
			slog.Error("cleanup", "error", err)
			return
		}
		rowsAffected, err := res.RowsAffected()
		if err != nil {
			slog.Error("cleanup rowsAffected", "error", err)
			return
		}
		if rowsAffected == 0 {
			return
		}
	}
}
