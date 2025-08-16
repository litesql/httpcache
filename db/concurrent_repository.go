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

type concurrentRepository struct {
	db         *sql.DB
	queriers   []*querier
	globalQuit chan struct{}

	writers []*sql.Stmt
	// roundRobin strategy to choose one writer
	currentWriter int
	muWriter      sync.Mutex

	// workers for MultiDatabaseRepository
	source chan work
	quit   chan struct{}

	// cleanup attributes
	cleanupCancelation func()
	ttl                int64 // time to live in miliseconds
	cleaners           []*sql.Stmt
}

func newConcurrentRepository(db *sql.DB, databaseID int, ttl time.Duration, cleanupInterval time.Duration, tableNames ...string) (*concurrentRepository, error) {
	globalQuit := make(chan struct{})

	size := len(tableNames)
	queriers := make([]*querier, size)
	writers := make([]*sql.Stmt, size)
	cleaners := make([]*sql.Stmt, size)
	for i, tableName := range tableNames {
		readStmt, err := db.Prepare(readerQuery(tableName))
		if err != nil {
			return nil, fmt.Errorf("prepare read query for %q: %w", tableName, err)
		}
		q := newQuerier(readStmt, databaseID, tableName)
		queriers[i] = q
		q.start()

		writeStmt, err := db.Prepare(WriterQuery(tableName))
		if err != nil {
			return nil, fmt.Errorf("prepare writer query for %q: %w", tableName, err)
		}
		writers[i] = writeStmt

		cleanupStmt, err := db.Prepare(cleanupByTTLQuery(tableName))
		if err != nil {
			return nil, fmt.Errorf("prepare cleanup query for %q: %w", tableName, err)
		}

		cleaners[i] = cleanupStmt
	}

	repository := concurrentRepository{
		db:         db,
		queriers:   queriers,
		globalQuit: globalQuit,
		writers:    writers,
		cleaners:   cleaners,
		ttl:        int64(ttl.Seconds()),
	}
	if cleanupInterval > 0 && ttl > 0 {
		slog.Info("Repository cleanup started", "ttl", ttl, "interval", cleanupInterval)
		ctx, cancel := context.WithCancel(context.Background())
		repository.cleanupCancelation = cancel
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

func (r *concurrentRepository) FindByURL(ctx context.Context, url string) (*Response, error) {
	size := len(r.queriers)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	respCh := make(chan *Response, size)
	unitWork := work{
		ctx:  ctx,
		url:  url,
		resp: respCh,
	}

	go func() {
		for _, q := range r.queriers {
			q.source <- unitWork
		}
	}()
	var count int
	for {
		select {
		case resp := <-respCh:
			count++
			if resp != nil {
				return resp, nil
			}
			if count == size {
				return nil, sql.ErrNoRows
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (r *concurrentRepository) Write(ctx context.Context, url string, resp *Response) error {
	r.muWriter.Lock()
	defer r.muWriter.Unlock()
	r.currentWriter++
	if r.currentWriter >= len(r.writers) {
		r.currentWriter = 0
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: false})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	if resp.TableName != "" {
		_, err = tx.ExecContext(ctx, "DELETE FROM "+resp.TableName+" WHERE url = ?", url)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	err = execWriter(ctx, tx.StmtContext(ctx, r.writers[r.currentWriter]), url, resp)
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (r *concurrentRepository) Close() error {
	if r.cleanupCancelation != nil {
		r.cleanupCancelation()
	}
	if r.quit != nil {
		close(r.quit)
	}
	close(r.globalQuit)
	var err error
	for _, stmt := range r.writers {
		err = errors.Join(stmt.Close())
	}
	for _, stmt := range r.cleaners {
		err = errors.Join(stmt.Close())
	}
	return err
}

func (r *concurrentRepository) cleanup() {
	r.muWriter.Lock()
	defer r.muWriter.Unlock()
	for _, cleanupStmt := range r.cleaners {
		for {
			res, err := cleanupStmt.Exec(r.ttl)
			if err != nil {
				slog.Error("cleanup", "error", err)
				break
			}
			rowsAffected, err := res.RowsAffected()
			if err != nil {
				slog.Error("cleanup rowsAffected", "error", err)
				break
			}
			if rowsAffected == 0 {
				break
			}
		}
	}
}

func (r *concurrentRepository) start() {
	r.source = make(chan work, 10)
	go func() {
		for {
			select {
			case unitWork := <-r.source:
				resp, err := r.FindByURL(unitWork.ctx, unitWork.url)
				if err != nil {
					unitWork.resp <- nil
					continue
				}
				unitWork.resp <- resp
			case <-r.quit:
				return
			}
		}
	}()
}

type work struct {
	ctx  context.Context
	url  string
	resp chan *Response
}

type querier struct {
	source     chan work
	quit       chan struct{}
	stmt       *sql.Stmt
	tableName  string
	databaseID int
}

func newQuerier(stmt *sql.Stmt, databaseID int, tableName string) *querier {
	return &querier{
		stmt:       stmt,
		tableName:  tableName,
		databaseID: databaseID,
	}
}

func (q *querier) start() {
	q.source = make(chan work, 10)
	go func() {
		for {
			select {
			case unitWork := <-q.source:
				response, err := rowToResponse(q.stmt.QueryRowContext(unitWork.ctx, unitWork.url))
				if err != nil {
					unitWork.resp <- nil
					continue
				}
				response.TableName = q.tableName
				response.DatabaseID = q.databaseID
				unitWork.resp <- response

			case <-q.quit:
				q.stmt.Close()
				return
			}
		}
	}()
}
