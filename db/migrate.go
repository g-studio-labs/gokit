package db

import (
	"database/sql"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql "pgx" driver, for goose
	"github.com/pressly/goose/v3"

	"github.com/g-studio-labs/gokit/lg"
)

// Migrate applies the goose migrations in migrations (an embed.FS with the
// *.sql in dir, "." if they are at the root) to dsn. It runs with the schema
// owner role (MIGRATIONS_DATABASE_URL), from `api migrate` or the deploy job,
// never at server startup.
//
//	//go:embed *.sql
//	var FS embed.FS
//	...
//	db.Migrate(dsn, migrations.FS, ".")
func Migrate(dsn string, migrations fs.FS, dir string) error {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open database to migrate: %w", err)
	}
	defer conn.Close()
	if dir == "" {
		dir = "."
	}
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	goose.SetLogger(gooseLogger{})
	return goose.Up(conn, dir)
}

// Status prints the state of each migration (applied or pending).
func Status(dsn string, migrations fs.FS, dir string) error {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer conn.Close()
	if dir == "" {
		dir = "."
	}
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Status(conn, dir)
}

type gooseLogger struct{}

func (gooseLogger) Fatalf(format string, v ...any) { panic(fmt.Sprintf(format, v...)) }
func (gooseLogger) Printf(format string, v ...any) {
	// goose ends its lines with \n; lg already adds it.
	msg := fmt.Sprintf(format, v...)
	if n := len(msg); n > 0 && msg[n-1] == '\n' {
		msg = msg[:n-1]
	}
	lg.Info(msg)
}
