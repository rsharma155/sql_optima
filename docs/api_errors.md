# API Error Handling (P2-4)

<!-- Author: Ravi Sharma -->
<!-- Copyright (c) 2026 Ravi Sharma -->
<!-- SPDX-License-Identifier: MIT -->

## Policy

- **Clients** receive short, stable `error` strings (no stack traces, SQL, or connection details).
- **Servers** log full errors with `slog` (JSON on stdout) including the wrapped `err` attribute.
- Use `internal/apiresponse` helpers in new and touched handlers:

```go
import "github.com/rsharma155/sql_optima/internal/apiresponse"

apiresponse.WriteJSONError(w, http.StatusInternalServerError, "failed to load data", err, "handler", "GetStatus")
apiresponse.WritePlainError(w, http.StatusInternalServerError, "failed to load data", err)
```

## v1.0 status

| Area | Status |
|------|--------|
| OS metrics ingest, cold storage, admin user APIs | Sanitized responses |
| Legacy monitoring handlers (widgets, storage index, explain) | May still return `err.Error()` — migrate incrementally |
| OpenTelemetry | Optional via `OTEL_EXPORTER_OTLP_ENDPOINT` |

## Roadmap

- Central middleware to map known error types (timeout, not found) to HTTP status + public message.
- Redact DSN/password substrings in slog attributes (never log request bodies for admin routes).
