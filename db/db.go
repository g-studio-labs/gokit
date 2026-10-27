// Package db opens GORM over Postgres with the house's pool and logger, and
// runs goose migrations embedded in the binary.
//
// Migrations do NOT run when the server starts: they go in a subcommand
// (`api migrate`) run with the schema owner role before switching the image.
// The server connects with a least-privilege role and never uses AutoMigrate
// outside development.
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/g-studio-labs/gokit/lg"
)

// Options tunes the connection. Zero values take the defaults.
type Options struct {
	MaxOpen         int           // 20
	MaxIdle         int           // 10
	ConnMaxIdleTime time.Duration // 5m
	ConnMaxLifetime time.Duration // 30m
	SlowThreshold   time.Duration // 200ms: slower queries go to Warn
	Retries         int           // 10 ping attempts at startup (the database may be slow to come up)
	// Debug logs every query (DB_DEBUG=1 locally).
	Debug bool
}

// Open connects, configures the pool and pings with retries.
func Open(ctx context.Context, dsn string, o Options) (*gorm.DB, error) {
	def := func(n *int, v int) {
		if *n == 0 {
			*n = v
		}
	}
	defD := func(d *time.Duration, v time.Duration) {
		if *d == 0 {
			*d = v
		}
	}
	def(&o.MaxOpen, 20)
	def(&o.MaxIdle, 10)
	def(&o.Retries, 10)
	defD(&o.ConnMaxIdleTime, 5*time.Minute)
	defD(&o.ConnMaxLifetime, 30*time.Minute)
	defD(&o.SlowThreshold, 200*time.Millisecond)

	level := gormlogger.Warn
	if o.Debug {
		level = gormlogger.Info
	}
	g, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: &slogAdapter{level: level, slow: o.SlowThreshold},
		// Each Create/Update does not need its own implicit transaction; the
		// ones that matter are opened explicitly with db.Transaction.
		SkipDefaultTransaction: true,
		// Driver errors are translated to gorm's (ErrDuplicatedKey...).
		TranslateError: true,
		// Dates in UTC: what goes out as JSON does not depend on the container's TZ.
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	sqlDB, err := g.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(o.MaxOpen)
	sqlDB.SetMaxIdleConns(o.MaxIdle)
	sqlDB.SetConnMaxIdleTime(o.ConnMaxIdleTime)
	sqlDB.SetConnMaxLifetime(o.ConnMaxLifetime)

	for i := 1; ; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = sqlDB.PingContext(pingCtx)
		cancel()
		if err == nil {
			break
		}
		if i >= o.Retries {
			return nil, fmt.Errorf("database not responding after %d attempts: %w", i, err)
		}
		lg.Warn("database not responding, retrying", "attempt", i, lg.Err(err))
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return g, nil
}

// NotFound is errors.Is(err, gorm.ErrRecordNotFound).
func NotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

// Duplicated is errors.Is(err, gorm.ErrDuplicatedKey) (with TranslateError).
func Duplicated(err error) bool { return errors.Is(err, gorm.ErrDuplicatedKey) }

// slogAdapter sends gorm's log through lg.
type slogAdapter struct {
	level gormlogger.LogLevel
	slow  time.Duration
}

func (l *slogAdapter) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	c := *l
	c.level = level
	return &c
}

func (l *slogAdapter) Info(ctx context.Context, msg string, args ...any) {
	if l.level >= gormlogger.Info {
		lg.Ctx(ctx).Info(fmt.Sprintf(msg, args...))
	}
}

func (l *slogAdapter) Warn(ctx context.Context, msg string, args ...any) {
	if l.level >= gormlogger.Warn {
		lg.Ctx(ctx).Warn(fmt.Sprintf(msg, args...))
	}
}

func (l *slogAdapter) Error(ctx context.Context, msg string, args ...any) {
	if l.level >= gormlogger.Error {
		lg.Ctx(ctx).Error(fmt.Sprintf(msg, args...))
	}
}

func (l *slogAdapter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level <= gormlogger.Silent {
		return
	}
	elapsed := time.Since(begin)
	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && l.level >= gormlogger.Error:
		sql, rows := fc()
		lg.Ctx(ctx).Error("sql", lg.Err(err), "ms", elapsed.Milliseconds(), "rows", rows, "query", sql)
	case l.slow > 0 && elapsed > l.slow && l.level >= gormlogger.Warn:
		sql, rows := fc()
		lg.Ctx(ctx).Warn("slow sql", "ms", elapsed.Milliseconds(), "rows", rows, "query", sql)
	case l.level >= gormlogger.Info:
		sql, rows := fc()
		lg.Ctx(ctx).Debug("sql", "ms", elapsed.Milliseconds(), "rows", rows, "query", sql)
	}
}
