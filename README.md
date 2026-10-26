# gokit

Las piezas comunes de las APIs en Go de la casa (dwit, nuke-ai, guz-studio),
en un módulo: banner, logger, sobre de respuesta, middlewares, servidor,
configuración, auth y base de datos. Cada paquete se usa por separado; el que
no importa `db` no compila GORM.

```sh
go get github.com/RomanshkVolkov/gokit@latest
```

La skill `go-api` de Claude Code (`~/.claude/skills/go-api`) explica cuándo
usar cada pieza y arma proyectos nuevos con ellas.

## Un `main.go` completo

```go
package main

import (
	"context"
	"net/http"
	"os"

	"github.com/RomanshkVolkov/gokit/banner"
	"github.com/RomanshkVolkov/gokit/config"
	"github.com/RomanshkVolkov/gokit/httpserver"
	"github.com/RomanshkVolkov/gokit/lg"
	"github.com/RomanshkVolkov/gokit/middleware"
	"github.com/RomanshkVolkov/gokit/response"
)

func main() {
	config.LoadDotEnv()
	lg.Setup(lg.Options{Service: "demo-api"})
	cfg := struct {
		Addr    string
		Origins []string
	}{
		Addr:    config.Default("API_ADDR", ":8080"),
		Origins: config.List("CORS_ORIGINS"),
	}
	config.Must()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		response.Message(w, r, "ok")
	})
	mux.Handle("GET /items/{id}", response.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := response.PathInt(r, "id")
		if err != nil {
			return err
		}
		if id > 100 {
			return response.NotFound("")
		}
		response.OK(w, r, map[string]int{"id": id})
		return nil
	}))

	h := middleware.Chain(mux,
		middleware.Recover,
		middleware.RequestID,
		middleware.RealIP(),
		middleware.Logger(middleware.LoggerOptions{}),
		middleware.SecurityHeaders(middleware.HeaderOptions{}),
		middleware.CORS(middleware.CORSOptions{Origins: cfg.Origins}),
		middleware.MaxBody(1<<20),
	)

	srv := httpserver.New(cfg.Addr, h, httpserver.Options{
		BeforeListen: func() {
			banner.Print(banner.Options{Name: "DEMO", Subtitle: "API", Accent: banner.Hex("#3B82F6"), Env: config.Env(), Addr: cfg.Addr})
		},
	})
	if err := srv.Run(context.Background()); err != nil {
		lg.Error("servidor", lg.Err(err))
		os.Exit(1)
	}
}
```

## El sobre

```json
{ "success": true,  "data": {...}, "message": "Creado", "meta": {"page":1,"pageSize":20,"total":57,"totalPages":3}, "requestId": "4f2c…" }
{ "success": false, "error": "validation", "message": "Datos no válidos", "fields": [{"field":"email","message":"es obligatorio","rule":"required"}], "requestId": "4f2c…" }
```

`error` es un código estable (`not-found`, `validation`, `unauthorized`,
`forbidden`, `conflict`, `rate-limited`, `internal`, …); el cliente decide por
él, nunca por `message`. El status HTTP siempre coincide con `success`.

## Paquetes

| Paquete | Qué trae |
|---|---|
| `banner` | `Print(Options{Name, Subtitle, Accent, Env, Addr, Rows})`, `Hex`, `Wordmark` |
| `lg` | `Setup`, `Info/Warn/Error/Debug(msg, attrs...)`, `*f`, `Ctx(ctx)`, `Err`, `Secret` |
| `response` | `Response[T]`, `*Error` + constructores, `OK/Created/Paged/Fail/Respond/ToolReply`, `H`, `Decode[T]`, `Validate`, `PathInt`, `Paginate` |
| `middleware` | `Chain`, `RequestID`, `RealIP`, `ClientIP`, `Logger`, `Recover`, `CORS`, `AntiCSRF`, `SecurityHeaders`, `MaxBody`, `Timeout`, `RateLimit` |
| `httpserver` | `New(addr, h, Options)`, `Run(ctx)`, `HealthCheck` |
| `config` | `Get/Required/Default/Bool/Int/Duration/List`, `Must`, `LoadDotEnv`, `Env`, `Mask` |
| `auth` | `NewSigner`, `Sign`, `Parse[T]`, `Bearer[T]`, `Cookie[T]`, `RequireRole`, `HashPassword`, `VerifyPassword`, `NewAPIKey`, `RequireAPIKey`, `SessionCookie` |
| `db` | `Open(ctx, dsn, Options)`, `Migrate(dsn, fs, dir)`, `Status`, `NotFound`, `Duplicated` |

## Desarrollo

```sh
go test -race ./...
go vet ./... && gofmt -l .
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Versionado semántico; un cambio que rompa la API pública sube la mayor. Los
códigos de `response` son contrato con los clientes: no se renombran.
