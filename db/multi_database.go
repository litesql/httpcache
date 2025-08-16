package db

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"
)

type MultiDatabaseRepository struct {
	concurrentRepositories []*concurrentRepository
	// roundRobin strategy to choose one writer
	currentWriter int
	muWriter      sync.Mutex
}

func NewMultiDatabaseRepositoryWithTTL(ttl time.Duration, cleanupInterval time.Duration, dbs []*sql.DB) (*MultiDatabaseRepository, error) {
	concurrentRepositories := make([]*concurrentRepository, len(dbs))
	for i, db := range dbs {
		tables, err := ResponseTables(db)
		if err != nil {
			return nil, err
		}
		cr, err := newConcurrentRepository(db, i, ttl, cleanupInterval, tables...)
		if err != nil {
			return nil, err
		}
		cr.start()
		concurrentRepositories[i] = cr
	}
	return &MultiDatabaseRepository{
		concurrentRepositories: concurrentRepositories,
	}, nil
}

func NewMultiDatabaseRepository(dbs []*sql.DB) (*MultiDatabaseRepository, error) {
	return NewMultiDatabaseRepositoryWithTTL(0, 0, dbs)
}

func (r *MultiDatabaseRepository) FindByURL(ctx context.Context, url string) (*Response, error) {
	size := len(r.concurrentRepositories)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	respCh := make(chan *Response, size)
	unitWork := work{
		ctx:  ctx,
		url:  url,
		resp: respCh,
	}

	go func() {
		for _, cr := range r.concurrentRepositories {
			cr.source <- unitWork
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

func (r *MultiDatabaseRepository) Write(ctx context.Context, url string, resp *Response) error {
	var cr Repository
	if resp.DatabaseID == -1 {
		r.muWriter.Lock()
		r.currentWriter++
		if r.currentWriter >= len(r.concurrentRepositories) {
			r.currentWriter = 0
		}
		cr = r.concurrentRepositories[r.currentWriter]
		r.muWriter.Unlock()
	} else {
		cr = r.concurrentRepositories[resp.DatabaseID]
	}
	return cr.Write(ctx, url, resp)
}

func (r *MultiDatabaseRepository) Close() error {
	var err error
	for _, cr := range r.concurrentRepositories {
		err = errors.Join(err, cr.Close())
	}
	return err
}
