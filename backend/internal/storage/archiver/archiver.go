// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Time-series data archiver for purging old metrics based on retention policies.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package archiver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type Config struct {
	ArchivePath string
	BatchSize   int
	RunInterval time.Duration
}

func DefaultConfig() *Config {
	return &Config{
		ArchivePath: getEnv("ARCHIVE_DATA_PATH", "./data/archive"),
		BatchSize:   10000,
		RunInterval: 24 * time.Hour,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

type Archiver struct {
	hotStorage *hot.HotStorage
	config     *Config
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

func New(hs *hot.HotStorage, cfg *Config) *Archiver {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Archiver{
		hotStorage: hs,
		config:     cfg,
		stopCh:     make(chan struct{}),
	}
}

func (a *Archiver) Start(ctx context.Context) {
	a.wg.Add(1)
	go a.runScheduler(ctx)
	log.Println("[Archiver] Started nightly archive scheduler")
}

func (a *Archiver) Stop() {
	close(a.stopCh)
	a.wg.Wait()
	log.Println("[Archiver] Stopped")
}

func (a *Archiver) runScheduler(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.RunInterval)
	defer ticker.Stop()

	a.runArchive(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.runArchive(ctx)
		}
	}
}

func (a *Archiver) runArchive(ctx context.Context) {
	cutoff := time.Now().AddDate(0, 0, -30)

	log.Printf("[Archiver] Starting archive for data older than %s", cutoff.Format(time.RFC3339))

	archivedCount := 0
	for {
		metrics, servers, err := a.hotStorage.GetMetricsForArchive(ctx, cutoff, a.config.BatchSize)
		if err != nil {
			log.Printf("[Archiver] Error fetching metrics: %v", err)
			return
		}

		if len(metrics) == 0 {
			break
		}

		grouped := a.groupByDateAndServer(metrics)

		for _, batch := range grouped {
			date := batch[0].CaptureTimestamp.Format("2006-01-02")
			serverName := batch[0].ServerName

			if err := a.writeParquetFile(serverName, date, batch); err != nil {
				log.Printf("[Archiver] Error writing parquet file: %v", err)
				continue
			}

			archivedCount += len(batch)
		}

		log.Printf("[Archiver] Archived batch of %d metrics (servers: %s)", len(metrics), servers)

		if len(metrics) < a.config.BatchSize {
			break
		}
	}

	if archivedCount > 0 {
		log.Printf("[Archiver] Successfully archived %d metrics, dropping chunks older than 30 days", archivedCount)
		if err := a.hotStorage.DeleteChunksOlderThan(ctx, 30*24*time.Hour); err != nil {
			log.Printf("[Archiver] Warning: Failed to drop old chunks: %v", err)
		}
	} else {
		log.Println("[Archiver] No data to archive")
	}
}

func (a *Archiver) groupByDateAndServer(metrics []*hot.Metric) map[string][]*hot.Metric {
	grouped := make(map[string][]*hot.Metric)

	for _, m := range metrics {
		date := m.CaptureTimestamp.Format("2006-01-02")
		key := fmt.Sprintf("%s|%s", date, m.ServerName)
		grouped[key] = append(grouped[key], m)
	}

	return grouped
}

func (a *Archiver) writeParquetFile(serverName, date string, metrics []*hot.Metric) error {
	dir := filepath.Join(
		a.config.ArchivePath,
		fmt.Sprintf("server_name=%s", serverName),
		fmt.Sprintf("date=%s", date),
	)

	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	parquetPath := filepath.Join(dir, "metrics.parquet")

	rows := make([]parquetMetricRow, len(metrics))
	for i, m := range metrics {
		tags, _ := json.Marshal(m.Tags)
		rows[i] = parquetMetricRow{
			CaptureTimestamp: m.CaptureTimestamp.UnixMilli(),
			ServerName:       m.ServerName,
			MetricName:       m.MetricName,
			MetricValue:      m.MetricValue,
			Tags:             string(tags),
		}
	}

	f, err := os.Create(parquetPath)
	if err != nil {
		return fmt.Errorf("failed to create parquet file: %w", err)
	}
	defer f.Close()

	writer := parquet.NewGenericWriter[parquetMetricRow](f)
	defer writer.Close()

	_, err = writer.Write(rows)
	if err != nil {
		return fmt.Errorf("failed to write parquet: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close parquet writer: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	if err := a.verifyParquetFile(parquetPath, len(rows)); err != nil {
		_ = os.Remove(parquetPath)
		return fmt.Errorf("parquet verification failed: %w", err)
	}

	log.Printf("[Archiver] Wrote %s (%d rows)", parquetPath, len(rows))
	return nil
}

func (a *Archiver) verifyParquetFile(path string, expectedRows int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := parquet.NewGenericReader[parquetMetricRow](f)
	defer reader.Close()

	if reader.NumRows() != int64(expectedRows) {
		return fmt.Errorf("row count mismatch: expected %d, got %d", expectedRows, reader.NumRows())
	}

	return nil
}

type parquetMetricRow struct {
	CaptureTimestamp int64   `parquet:"name=capture_timestamp, type=INT64"`
	ServerName       string  `parquet:"name=server_name, type=BYTE_ARRAY, converted=STRING"`
	MetricName       string  `parquet:"name=metric_name, type=BYTE_ARRAY, converted=STRING"`
	MetricValue      float64 `parquet:"name=metric_value, type=DOUBLE"`
	Tags             string  `parquet:"name=tags, type=BYTE_ARRAY, converted=STRING"`
}

func (a *Archiver) RunOnce(ctx context.Context) error {
	a.runArchive(ctx)
	return nil
}
