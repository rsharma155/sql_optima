# SQL Optima API + SPA. Build from repository root:
#   docker build -t sql-optima:local .
# Same image is published to GHCR on v*.*.* tags and used by docker/docker-compose.yml.
#
# Debian (not distroless): wget for Compose healthchecks, shell entrypoint to load
# Vault's token from the vault_data volume (VAULT_TOKEN_FILE).
FROM golang:1.26-bookworm AS build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
COPY backend/internal/sqlplan-analyzer ./internal/sqlplan-analyzer
RUN go mod download
COPY backend/ ./
COPY infrastructure/sql_scripts /src/infrastructure/sql_scripts
# config.yaml is optional — instances come from server registry in Docker mode.
RUN echo 'instances: []' > ../config.yaml
COPY frontend /src/frontend
RUN CGO_ENABLED=0 go build -mod=mod -ldflags="-s -w" -o /sql-optima ./cmd/server
RUN mkdir -p /src/backend/logs && chmod 0777 /src/backend/logs

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates wget \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /srv
ENV SQL_OPTIMA_SQL_SCRIPTS_DIR=/srv/sql_scripts
COPY --from=build /src/infrastructure/sql_scripts /srv/sql_scripts
COPY --from=build /src/config.yaml ./
COPY --from=build /src/frontend ./frontend
COPY --from=build /src/backend/internal/intel/templates ./backend/internal/intel/templates
COPY --from=build /sql-optima ./backend/sql-optima
COPY docker/scripts/api-entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh
WORKDIR /srv/backend
COPY --from=build /src/backend/logs ./logs
EXPOSE 8080
ENV PORT=:8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["/entrypoint.sh"]
