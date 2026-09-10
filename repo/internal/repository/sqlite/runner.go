package sqlite

import (
	"context"
	"database/sql"
)

type queryRowRunner interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type queryRunner interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type execRunner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type queryExecRunner interface {
	queryRunner
	execRunner
}
