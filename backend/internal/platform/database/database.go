package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	appmigrations "github.com/jfxdev/grom/backend/migrations"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/migrate"
)

type Kind string

const (
	SQLite   Kind = "sqlite"
	Postgres Kind = "postgres"
)

var sqliteMigrationMu sync.Mutex

func Open(ctx context.Context, databaseURL string) (*bun.DB, Kind, error) {
	switch {
	case strings.HasPrefix(databaseURL, "sqlite://"):
		dsn := strings.TrimPrefix(databaseURL, "sqlite://")
		if dsn == "" {
			return nil, "", fmt.Errorf("sqlite database path is empty")
		}
		if dsn != ":memory:" && !strings.HasPrefix(dsn, "file:") {
			if err := os.MkdirAll(filepath.Dir(dsn), 0o750); err != nil {
				return nil, "", fmt.Errorf("create sqlite directory: %w", err)
			}
		}
		separator := "?"
		if strings.Contains(dsn, "?") {
			separator = "&"
		}
		sqlDB, err := sql.Open(sqliteshim.ShimName, dsn+separator+"_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
		if err != nil {
			return nil, "", fmt.Errorf("open sqlite: %w", err)
		}
		db := bun.NewDB(sqlDB, sqlitedialect.New())
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, "", fmt.Errorf("ping sqlite: %w", err)
		}
		return db, SQLite, nil
	case strings.HasPrefix(databaseURL, "postgres://"), strings.HasPrefix(databaseURL, "postgresql://"):
		if _, err := url.Parse(databaseURL); err != nil {
			return nil, "", fmt.Errorf("parse postgres URL: %w", err)
		}
		sqlDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(databaseURL)))
		db := bun.NewDB(sqlDB, pgdialect.New())
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, "", fmt.Errorf("ping postgres: %w", err)
		}
		return db, Postgres, nil
	default:
		return nil, "", fmt.Errorf("unsupported database URL scheme")
	}
}

func Migrate(ctx context.Context, db *bun.DB, kind Kind, lockWait time.Duration, logger *slog.Logger) error {
	return migrateDatabase(ctx, db, kind, lockWait, logger, appmigrations.Collection)
}

func migrateDatabase(
	ctx context.Context,
	db *bun.DB,
	kind Kind,
	lockWait time.Duration,
	logger *slog.Logger,
	collection *migrate.Migrations,
) error {
	unlock, err := migrationLock(ctx, db, kind, lockWait)
	if err != nil {
		return err
	}
	defer unlock()

	migrator := migrate.NewMigrator(db, collection, migrate.WithMarkAppliedOnSuccess(true))
	if err := migrator.Init(ctx); err != nil {
		return fmt.Errorf("initialize migration history: %w", err)
	}

	started := time.Now()
	group, err := migrator.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	logger.Info("database migrations complete", "group", group.String(), "duration", time.Since(started))
	return nil
}

func Checkpoint(ctx context.Context, db *bun.DB, kind Kind) error {
	if kind != SQLite {
		return fmt.Errorf("integrated backup supports SQLite only")
	}
	var busy, logFrames, checkpointedFrames int
	if err := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").
		Scan(&busy, &logFrames, &checkpointedFrames); err != nil {
		return fmt.Errorf("checkpoint sqlite database: %w", err)
	}
	if busy != 0 {
		return fmt.Errorf("checkpoint sqlite database: database remained busy")
	}
	return nil
}

func migrationLock(ctx context.Context, db *bun.DB, kind Kind, wait time.Duration) (func(), error) {
	if kind == SQLite {
		sqliteMigrationMu.Lock()
		return sqliteMigrationMu.Unlock, nil
	}

	lockCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	conn, err := db.Conn(lockCtx)
	if err != nil {
		return nil, fmt.Errorf("acquire postgres migration connection: %w", err)
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		var acquired bool
		if err := conn.QueryRowContext(
			lockCtx, "SELECT pg_try_advisory_lock(?)", int64(0x47524f4d),
		).Scan(&acquired); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("acquire postgres migration lock: %w", err)
		}
		if acquired {
			return func() {
				defer func() { _ = conn.Close() }()
				unlockCtx, unlockCancel := context.WithTimeout(context.WithoutCancel(ctx), wait)
				defer unlockCancel()
				_, _ = conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock(?)", int64(0x47524f4d))
			}, nil
		}
		select {
		case <-lockCtx.Done():
			_ = conn.Close()
			return nil, fmt.Errorf("migration lock timeout: %w", lockCtx.Err())
		case <-ticker.C:
		}
	}
}
