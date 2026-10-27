# Changelog

## v0.2.0 · 2026-10-26

Todo el código en inglés: comentarios, godoc, logs, errores internos y tests.
La API pública no cambia.

- **Logs en inglés** (p. ej. `missing environment variables: …`,
  `could not serialize the response`, `panic in handler`, `slow sql`).
- **Atributos de log renombrados:** `intento` → `attempt`, `causa` → `cause`.
  Revisar dashboards o alertas que filtren por esos campos.
- **Siguen en español** los textos para el usuario final: `response.Messages`,
  `response.RuleMessages` y los mensajes que llegan a `message`/`fields[].message`
  del sobre (validación, `Origen no permitido`, `Petición cancelada`…). Un
  proyecto los cambia al arrancar.
- Doc de `config.Bool` corregido: un valor no reconocido devuelve `def` y lo
  reporta `Must`.

## v0.1.0 · 2026-10-26

Primera versión. Extrae a un módulo lo que varios servicios llevaban copiado:

- `banner`: splash ANSI Shadow con wordmark renderizado desde la fuente
  embebida, color de marca por hex, filas `[+]` y versión del binario.
- `lg`: misma API (`Info/Warn/Error`, `*f`) sobre `log/slog`; texto a color en
  terminal, JSON fuera; `LOG_LEVEL`, `LOG_FORMAT`; `request_id` por contexto.
- `response`: sobre `{success, message, data, error, fields, meta, requestId}`,
  `*Error` con status y código estable, `Decode[T]` estricto con validator,
  `H` para handlers que devuelven error, paginación.
- `middleware`: `Chain`, `RequestID`, `RealIP` (proxy privado + Cloudflare),
  `Logger` con redacción, `Recover`, `CORS` lista blanca, `AntiCSRF`,
  `SecurityHeaders`, `MaxBody`, `Timeout`, `RateLimit`.
- `httpserver`: `http.Server` con timeouts y apagado ordenado por señal;
  `HealthCheck` para el `HEALTHCHECK` de distroless.
- `config`: env → `NAME_FILE` → `/run/secrets`, `op://` sin resolver cuenta
  como faltante, `.env` que no pisa, `Must()` con la lista de faltantes.
- `auth`: JWT HS256 (`WithValidMethods`, emisor, exp), argon2id con
  compatibilidad pbkdf2/bcrypt, API keys con hash, cookies de sesión,
  `Bearer`/`Cookie`/`RequireRole`/`RequireAPIKey`.
- `db`: GORM Postgres con pool, logger por `lg` y `TranslateError`; `Migrate`
  con goose embebido.
