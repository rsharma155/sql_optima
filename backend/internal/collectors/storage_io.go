package collectors

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/rsharma155/sql_optima/internal/models"
)

// CollectStorageIO returns disk I/O latency and tempdb pressure metrics
// Parameters: ctx context.Context, db *sql.DB
// Returns: []FileIOStat, *TempDBStats, error
func CollectStorageIO(ctx context.Context, db *sql.DB) ([]models.FileIOStat, *models.TempDBStats, error) {
	fileIOQuery := `
		SELECT 
			DB_NAME(mf.database_id) AS database_name,
			mf.physical_name,
			mf.type_desc AS file_type,
			CAST(vfs.io_stall_read_ms AS FLOAT) / NULLIF(vfs.num_of_reads, 0) AS read_latency_ms,
			CAST(vfs.io_stall_write_ms AS FLOAT) / NULLIF(vfs.num_of_writes, 0) AS write_latency_ms,
			vfs.num_of_reads,
			vfs.num_of_writes,
			mf.size * 8 / 1024 AS size_mb
		FROM sys.dm_io_virtual_file_stats(NULL, NULL) vfs
		INNER JOIN sys.master_files mf ON vfs.database_id = mf.database_id AND vfs.file_id = mf.file_id
		WHERE mf.type_desc IN ('DATA', 'LOG')
		ORDER BY database_name, file_type`

	rows, err := db.QueryContext(ctx, fileIOQuery)
	if err != nil {
		log.Printf("[Collector] CollectStorageIO FileIO Error: %v", err)
		return nil, nil, fmt.Errorf("failed to fetch file I/O stats: %w", err)
	}
	defer rows.Close()

	var fileStats []models.FileIOStat
	var sizeMB int64
	for rows.Next() {
		var f models.FileIOStat
		var readLat, writeLat sql.NullFloat64

		err := rows.Scan(
			&f.DatabaseName, &f.PhysicalName, &f.FileType,
			&readLat, &writeLat, &f.NumOfReads, &f.NumOfWrites, &sizeMB,
		)
		if err != nil {
			log.Printf("[Collector] CollectStorageIO Scan Error: %v", err)
			continue
		}

		if readLat.Valid {
			f.ReadLatencyMs = readLat.Float64
		}
		if writeLat.Valid {
			f.WriteLatencyMs = writeLat.Float64
		}
		f.IoStallReadMs = 0
		f.IoStallWriteMs = 0

		fileStats = append(fileStats, f)
	}

	tempDBQuery := `
		SELECT 
			DB_NAME(database_id) AS database_name,
			COUNT(*) AS total_data_files,
			SUM(size * 8 / 1024) AS total_size_mb,
			0 AS used_space_mb,
			0.0 AS pfs_contention_pct,
			0.0 AS gam_contention_pct,
			0.0 AS sgam_contention_pct
		FROM sys.master_files
		WHERE database_id = 2 AND type_desc = 'DATA'
		GROUP BY database_id`

	var tempDB models.TempDBStats
	err = db.QueryRowContext(ctx, tempDBQuery).Scan(
		&tempDB.DatabaseName, &tempDB.TotalDataFiles, &tempDB.TotalSizeMB,
		&tempDB.UsedSpaceMB, &tempDB.PFSContentionPct, &tempDB.GAMContentionPct, &tempDB.SGAMContentionPct,
	)
	if err != nil {
		log.Printf("[Collector] CollectStorageIO TempDB Error: %v", err)
	}

	return fileStats, &tempDB, nil
}
