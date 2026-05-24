package wsstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	sqliteRetryAttempts  = 8
	sqliteRetryBaseDelay = 10 * time.Millisecond
	sqliteRetryMaxDelay  = 120 * time.Millisecond
)

func withSQLiteRetry(ctx context.Context, fn func() error) error {
	var err error
	delay := sqliteRetryBaseDelay
	for attempt := 0; attempt < sqliteRetryAttempts; attempt++ {
		err = fn()
		if !isSQLiteBusyOrLocked(err) {
			return err
		}
		if attempt == sqliteRetryAttempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		if delay < sqliteRetryMaxDelay {
			delay *= 2
			if delay > sqliteRetryMaxDelay {
				delay = sqliteRetryMaxDelay
			}
		}
	}
	return err
}

func withSQLiteResultRetry(ctx context.Context, fn func() (sql.Result, error)) (sql.Result, error) {
	var result sql.Result
	err := withSQLiteRetry(ctx, func() error {
		var err error
		result, err = fn()
		return err
	})
	return result, err
}

func isSQLiteBusyOrLocked(err error) bool {
	if err == nil {
		return false
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		code := sqliteErr.Code()
		primary := code & 0xff
		return primary == sqlite3.SQLITE_BUSY || primary == sqlite3.SQLITE_LOCKED
	}
	message := strings.ToUpper(err.Error())
	return strings.Contains(message, "SQLITE_BUSY") || strings.Contains(message, "SQLITE_LOCKED") || strings.Contains(message, "DATABASE IS LOCKED")
}
