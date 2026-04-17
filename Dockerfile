# Build from repository root: docker build -t sql-optima:latest .
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /src/backend
RUN go mod download
COPY backend/ ./
COPY infrastructure/sql_scripts /src/infrastructure/sql_scripts
# config.yaml is optional — instances come from server registry in Docker mode.
# Generate a safe default; users can volume-mount their own at runtime.
RUN echo 'instances: []' > ../config.yaml
COPY frontend ../frontend
# Force module mode even if a stale vendor/ exists in build context.
# GOTOOLCHAIN=auto lets the builder satisfy go.mod's minimum-version requirement
# even when the base image ships a slightly older patch release.
RUN GOTOOLCHAIN=auto CGO_ENABLED=0 go build -mod=mod -ldflags="-s -w" -o /sql-optima ./cmd/server
RUN mkdir -p /src/backend/logs && chmod 0777 /src/backend/logs

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /srv
ENV SQL_OPTIMA_SQL_SCRIPTS_DIR=/srv/sql_scripts
COPY --from=build /src/infrastructure/sql_scripts /srv/sql_scripts
COPY --from=build /src/config.yaml ./
COPY --from=build /src/frontend ./frontend
COPY --from=build /sql-optima ./backend/sql-optima
WORKDIR /srv/backend
COPY --from=build --chown=nonroot:nonroot /src/backend/logs ./logs
USER nonroot:nonroot
EXPOSE 8080
ENV PORT=:8080
ENTRYPOINT ["./sql-optima"]
