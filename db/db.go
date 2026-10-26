// Package db abre GORM sobre Postgres con el pool y el logger de la casa, y
// corre migraciones goose embebidas en el binario.
//
// Las migraciones NO corren al arrancar el servidor: van en un subcomando
// (`api migrate`) que se ejecuta con el rol dueño del esquema antes de
// cambiar la imagen. El servidor conecta con un rol de mínimos privilegios y
// nunca usa AutoMigrate fuera de desarrollo.
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/RomanshkVolkov/gokit/lg"
)

// Options ajusta la conexión. Los ceros toman los defaults.
type Options struct {
	MaxOpen         int           // 20
	MaxIdle         int           // 10
	ConnMaxIdleTime time.Duration // 5m
	ConnMaxLifetime time.Duration // 30m
	SlowThreshold   time.Duration // 200ms: consultas más lentas van a Warn
	Retries         int           // 10 intentos de ping al arrancar (la base puede tardar)
	// Debug registra cada consulta (DB_DEBUG=1 en local).
	Debug bool
}

// Open conecta, configura el pool y hace ping con reintentos.
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
		// Cada Create/Update no necesita su propia transacción implícita; las
		// que importan se abren explícitas con db.Transaction.
		SkipDefaultTransaction: true,
		// Los errores del driver se traducen a los de gorm (ErrDuplicatedKey...).
		TranslateError: true,
		// Fechas en UTC: lo que sale por JSON no depende del TZ del contenedor.
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("abrir la base: %w", err)
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
			return nil, fmt.Errorf("la base no responde tras %d intentos: %w", i, err)
		}
		lg.Warn("la base no responde, reintento", "intento", i, lg.Err(err))
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return g, nil
}

// NotFound es errors.Is(err, gorm.ErrRecordNotFound).
func NotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

// Duplicated es errors.Is(err, gorm.ErrDuplicatedKey) (con TranslateError).
func Duplicated(err error) bool { return errors.Is(err, gorm.ErrDuplicatedKey) }

// slogAdapter manda el log de gorm por lg.
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
		lg.Ctx(ctx).Warn("sql lenta", "ms", elapsed.Milliseconds(), "rows", rows, "query", sql)
	case l.level >= gormlogger.Info:
		sql, rows := fc()
		lg.Ctx(ctx).Debug("sql", "ms", elapsed.Milliseconds(), "rows", rows, "query", sql)
	}
}
