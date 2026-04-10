package collectors

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

const (
	LiveInterval       = 15 * time.Second
	HistoricalInterval = 60 * time.Second
)

type CollectorResult struct {
	CPU           *models.CPUTick
	Memory        *models.MemoryStats
	WaitStats     []models.WaitStat
	FileStats     []models.FileIOStat
	TempDBStats   *models.TempDBStats
	ActiveQueries []models.ActiveQuery
	LongRunning   []models.LongRunningQuery
	Blocking      []models.BlockingNode
	Errors        []error
}

type MSSQLCollector struct {
	conns      map[string]*sql.DB
	mu         sync.RWMutex
	result     CollectorResult
	liveTicker *time.Ticker
	histTicker *time.Ticker
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func NewMSSQLCollector(conns map[string]*sql.DB) *MSSQLCollector {
	ctx, cancel := context.WithCancel(context.Background())
	return &MSSQLCollector{
		conns:  conns,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (c *MSSQLCollector) Start() {
	c.liveTicker = time.NewTicker(LiveInterval)
	c.histTicker = time.NewTicker(HistoricalInterval)

	log.Printf("[Collector] Split-Speed Background Daemon starting...")
	log.Printf("[Collector]   - Live Diagnostics ticker: every %v", LiveInterval)
	log.Printf("[Collector]   - Historical Storage ticker: every %v", HistoricalInterval)
	log.Printf("[Collector]   - Live collectors: queries_active.go, blocking_locks.go (every 15s)")
	log.Printf("[Collector]   - Historical collectors: cpu_memory.go, waits.go, storage_io.go (every 60s)")

	go func() {
		for {
			select {
			case <-c.ctx.Done():
				log.Printf("[Collector] Background daemon shutting down")
				return

			case <-c.liveTicker.C:
				c.runLiveCollectors()

			case <-c.histTicker.C:
				c.runHistoricalCollectors()
			}
		}
	}()
}

func (c *MSSQLCollector) Stop() {
	c.cancel()
	c.liveTicker.Stop()
	c.histTicker.Stop()
	c.wg.Wait()
	log.Printf("[Collector] Stopped all collectors")
}

func (c *MSSQLCollector) GetResult() CollectorResult {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.result
}

func (c *MSSQLCollector) runLiveCollectors() {
	var wg sync.WaitGroup
	errors := []error{}

	c.mu.Lock()
	conns := c.conns
	c.mu.Unlock()

	for name, db := range conns {
		wg.Add(1)
		go func(instanceName string, db *sql.DB) {
			defer wg.Done()

			queries, err := CollectActiveQueries(c.ctx, db)
			if err != nil {
				log.Printf("[Collector] ERROR CollectActiveQueries for %s: %v", instanceName, err)
				errors = append(errors, fmt.Errorf("active queries: %w", err))
			} else {
				c.mu.Lock()
				c.result.ActiveQueries = queries
				c.mu.Unlock()
			}

			blocking, err := CollectBlockingLocks(c.ctx, db)
			if err != nil {
				log.Printf("[Collector] ERROR CollectBlockingLocks for %s: %v", instanceName, err)
				errors = append(errors, fmt.Errorf("blocking locks: %w", err))
			} else {
				c.mu.Lock()
				c.result.Blocking = blocking
				c.mu.Unlock()
			}
		}(name, db)
	}

	wg.Wait()

	c.mu.Lock()
	c.result.Errors = errors
	c.mu.Unlock()

	log.Printf("[Collector] Live tick complete - ActiveQueries: %d, Blocking: %d, Errors: %d",
		len(c.result.ActiveQueries), len(c.result.Blocking), len(errors))
}

func (c *MSSQLCollector) runHistoricalCollectors() {
	var wg sync.WaitGroup
	errors := []error{}

	c.mu.Lock()
	conns := c.conns
	c.mu.Unlock()

	for name, db := range conns {
		wg.Add(1)
		go func(instanceName string, db *sql.DB) {
			defer wg.Done()

			cpu, mem, err := CollectCPUMemory(c.ctx, db)
			if err != nil {
				log.Printf("[Collector] ERROR CollectCPUMemory for %s: %v", instanceName, err)
				errors = append(errors, fmt.Errorf("cpu/memory: %w", err))
			} else {
				c.mu.Lock()
				c.result.CPU = cpu
				c.result.Memory = mem
				c.mu.Unlock()
			}

			waits, err := CollectWaitStats(c.ctx, db)
			if err != nil {
				log.Printf("[Collector] ERROR CollectWaitStats for %s: %v", instanceName, err)
				errors = append(errors, fmt.Errorf("wait stats: %w", err))
			} else {
				c.mu.Lock()
				c.result.WaitStats = waits
				c.mu.Unlock()
			}

			storage, tempdb, err := CollectStorageIO(c.ctx, db)
			if err != nil {
				log.Printf("[Collector] ERROR CollectStorageIO for %s: %v", instanceName, err)
				errors = append(errors, fmt.Errorf("storage I/O: %w", err))
			} else {
				c.mu.Lock()
				c.result.FileStats = storage
				c.result.TempDBStats = tempdb
				c.mu.Unlock()
			}

			longRunning, err := CollectLongRunningQueries(c.ctx, db)
			if err != nil {
				log.Printf("[Collector] ERROR CollectLongRunningQueries for %s: %v", instanceName, err)
				errors = append(errors, fmt.Errorf("long running: %w", err))
			} else {
				c.mu.Lock()
				c.result.LongRunning = longRunning
				c.mu.Unlock()
			}
		}(name, db)
	}

	wg.Wait()

	c.mu.Lock()
	c.result.Errors = errors
	c.mu.Unlock()

	log.Printf("[Collector] Historical tick complete - CPU: %v, Memory: %v, Waits: %d, Storage: %d, TempDB: %v, LongRunning: %d, Errors: %d",
		c.result.CPU != nil, c.result.Memory != nil, len(c.result.WaitStats), len(c.result.FileStats),
		c.result.TempDBStats != nil, len(c.result.LongRunning), len(errors))
}
