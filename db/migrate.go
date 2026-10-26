package db

import (
	"database/sql"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib" // driver "pgx" de database/sql, para goose
	"github.com/pressly/goose/v3"

	"github.com/RomanshkVolkov/gokit/lg"
)

// Migrate aplica las migraciones goose de migrations (un embed.FS con los
// *.sql en dir, "." si están en la raíz) sobre dsn. Se corre con el rol
// dueño del esquema (MIGRATIONS_DATABASE_URL), desde `api migrate` o el job
// de deploy, nunca en el arranque del servidor.
//
//	//go:embed *.sql
//	var FS embed.FS
//	...
//	db.Migrate(dsn, migrations.FS, ".")
func Migrate(dsn string, migrations fs.FS, dir string) error {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("abrir la base para migrar: %w", err)
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

// Status imprime el estado de cada migración (aplicada o pendiente).
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
	// goose termina sus líneas con \n; lg ya lo pone.
	msg := fmt.Sprintf(format, v...)
	if n := len(msg); n > 0 && msg[n-1] == '\n' {
		msg = msg[:n-1]
	}
	lg.Info(msg)
}
