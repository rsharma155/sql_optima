# Cold Storage Implementation Plan — SQL Optima

This plan details the integration of a cold storage tier (MinIO/S3) for long-term metric retention using typed Parquet files and a robust export pipeline.

## 1. Complexity Assessment
The integration is **Moderately Complex**. While the architectural path is well-defined, it requires:
- Introducing new infrastructure (MinIO).
- Implementing a reliable, watermarked background export process.
- Moving from a generic EAV data model to typed schemas for each metric table.
- Federated query capabilities (Phase 2).

## 2. File Impact
Approximately **15-20 files** will be touched or created:

### 2.1 New Files
- `backend/internal/storage/cold/config.go`: Environment configuration.
- `backend/internal/storage/cold/s3uploader.go`: S3/MinIO client wrapper.
- `backend/internal/storage/cold/watermark.go`: Export progress tracking.
- `backend/internal/storage/cold/exporter.go`: Core export orchestrator.
- `backend/internal/storage/cold/schemas/*.go`: Typed Parquet definitions (one per table).
- `infrastructure/sql_scripts/07_cold_storage.sql`: Watermark and audit log tables.

### 2.2 Modified Files
- `backend/go.mod` / `go.sum`: Adding `aws-sdk-go-v2`.
- `backend/internal/appserver/appserver.go`: Wiring the new exporter into the system lifecycle.
- `docker/docker-compose.yml`: Adding MinIO and bucket-init services.
- `docker/.env.example`: Adding new configuration variables.
- `backend/internal/storage/archiver/archiver.go`: Potentially deprecated or refactored to use the new `cold` package.

## 3. Impact on Existing Flows
- **Zero Downtime:** The implementation is entirely asynchronous and does not affect the hot path (ingestion and live dashboard queries).
- **No Data Loss:** The watermarking system ensures that data is only marked as exported after successful upload to cold storage.
- **Resource Usage:** The export job is scheduled for off-peak hours (e.g., 02:00 UTC) to minimize impact on system performance.

## 4. Operational Overhead
- **Infrastructure:** MinIO adds a new container. While lightweight, it requires disk space for the "cold" data (though highly compressed).
- **Maintenance:** Adding new metrics to the system will now require defining a corresponding Parquet schema for cold storage.
- **Monitoring:** A new `cold_export_status` view is provided to monitor the health of the archival pipeline.

## 5. Implementation Roadmap (Phase 1)

### Step 1: Schema & Infrastructure
- [ ] Add `07_cold_storage.sql` to initialize watermarking tables.
- [ ] Update `docker-compose.yml` with MinIO and `minio-init` services.
- [ ] Add `COLD_STORAGE_*` variables to `.env.example`.

### Step 2: Core Cold Storage Package
- [ ] Implement `cold.Config` for environment parsing.
- [ ] Implement `cold.S3Uploader` using the AWS Go SDK.
- [ ] Implement `cold.WatermarkStore` for persistence.

### Step 3: Typed Schemas
- [ ] Create `backend/internal/storage/cold/schemas/` directory.
- [ ] Implement schemas for high-priority tables: `sqlserver_cpu_history`, `sqlserver_wait_history`, `postgres_wait_event_stats`, etc.

### Step 4: Exporter Orchestrator
- [ ] Implement the `Exporter` struct that ties together watermarks, uploader, and SQL queries.
- [ ] Implement generic `WriteTypedParquet` helper.
- [ ] Register core tables in the exporter.

### Step 5: Wiring & Validation
- [ ] Initialize and start the `Exporter` in `appserver.go`.
- [ ] Schedule nightly runs.
- [ ] Validate implementation using DuckDB queries against MinIO.

---
**Note:** Phase 2 (Iceberg/Trino) is excluded from this initial plan to focus on establishing a reliable data movement pipeline first.
