# Build from repository root: docker build -t sql-optima:latest .
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /src/backend
RUN go mod download
COPY backend/ ./
COPY config.yaml queries.yml ../
COPY frontend ../frontend
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /sql-optima ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /srv
COPY --from=build /src/config.yaml /src/queries.yml ./
COPY --from=build /src/frontend ./frontend
COPY --from=build /sql-optima ./backend/sql-optima
WORKDIR /srv/backend
USER nonroot:nonroot
EXPOSE 8080
ENV PORT=:8080
ENTRYPOINT ["./sql-optima"]
