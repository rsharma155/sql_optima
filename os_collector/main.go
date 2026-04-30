package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

type Config struct {
	BackendURL     string
	APIKey         string
	Interval       time.Duration
	InstanceName   string
}

type OSMetricsPayload struct {
	Timestamp    time.Time `json:"ts"`
	Hostname     string    `json:"hostname"`
	InstanceName string    `json:"instance_name"`

	// Memory
	TotalBytes     uint64 `json:"total_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	FreeBytes      uint64     `json:"free_bytes"`
	CachedBytes    uint64     `json:"cached_bytes"`
	BuffersBytes   uint64     `json:"buffers_bytes"`
	SharedBytes    uint64     `json:"shared_bytes"`
	SlabBytes      uint64     `json:"slab_bytes"`
	SwapTotalBytes uint64     `json:"swap_total_bytes"`
	SwapUsedBytes  uint64     `json:"swap_used_bytes"`
	SwapFreeBytes  uint64     `json:"swap_free_bytes"`
	DirtyBytes     uint64     `json:"dirty_bytes"`
	WritebackBytes uint64     `json:"writeback_bytes"`
	PageFaults     uint64     `json:"page_faults"`
	MajPageFaults  uint64     `json:"major_page_faults"`
	OOMKills       uint64     `json:"oom_kill_count"`

	// CPU
	CPUUserPct   float64 `json:"cpu_user_pct"`
	CPUSystemPct float64 `json:"cpu_system_pct"`
	CPUIdlePct   float64 `json:"cpu_idle_pct"`
	CPUIOWaitPct float64 `json:"cpu_iowait_pct"`
	Load1m       float64 `json:"load_1m"`
	Load5m       float64 `json:"load_5m"`
	Load15m      float64 `json:"load_15m"`
	CPUCores     int     `json:"cpu_cores"`

	// Postgres Process
	PostgresRSS     uint64 `json:"postgres_rss_bytes"`
	PostgresVSZ     uint64 `json:"postgres_vsz_bytes"`
	PostgresShared  uint64 `json:"postgres_shared_bytes"`
	PostgresPrivate uint64 `json:"postgres_private_bytes"`
	BackendCount    int    `json:"backend_count"`
}

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.BackendURL, "url", os.Getenv("SQL_OPTIMA_BACKEND_URL"), "Backend API URL (e.g. http://localhost:8080/api/os/metrics)")
	flag.StringVar(&cfg.APIKey, "key", os.Getenv("SQL_OPTIMA_API_KEY"), "API Key for authentication")
	flag.DurationVar(&cfg.Interval, "interval", 15*time.Second, "Collection interval")
	flag.StringVar(&cfg.InstanceName, "instance", os.Getenv("SQL_OPTIMA_INSTANCE_NAME"), "The name of the PG instance this collector is for")
	flag.Parse()

	if cfg.BackendURL == "" {
		log.Fatal("Backend URL is required. Use -url or SQL_OPTIMA_BACKEND_URL env var.")
	}

	hInfo, err := host.Info()
	if err != nil {
		log.Fatalf("Failed to get host info: %v", err)
	}

	hostname := hInfo.Hostname
	log.Printf("Starting OS Collector for instance %s on host %s", cfg.InstanceName, hostname)

	ticker := time.NewTicker(cfg.Interval)
	for range ticker.C {
		payload, err := collectMetrics(hostname, cfg.InstanceName)
		if err != nil {
			log.Printf("Error collecting metrics: %v", err)
			continue
		}

		if err := sendMetrics(cfg.BackendURL, cfg.APIKey, payload); err != nil {
			log.Printf("Error sending metrics: %v", err)
		}
	}
}

func collectMetrics(hostname, instanceName string) (*OSMetricsPayload, error) {
	p := &OSMetricsPayload{
		Timestamp:    time.Now().UTC(),
		Hostname:     hostname,
		InstanceName: instanceName,
	}

	// Memory
	v, err := mem.VirtualMemory()
	if err == nil {
		p.TotalBytes = v.Total
		p.AvailableBytes = v.Available
		p.UsedBytes = v.Used
		p.FreeBytes = v.Free
		p.CachedBytes = v.Cached
		p.BuffersBytes = v.Buffers
		p.SharedBytes = v.Shared
		p.SlabBytes = v.Slab
		p.DirtyBytes = v.Dirty
		p.WritebackBytes = v.WriteBack
	}

	s, err := mem.SwapMemory()
	if err == nil {
		p.SwapTotalBytes = s.Total
		p.SwapUsedBytes = s.Used
		p.SwapFreeBytes = s.Free
	}

	// CPU
	cpuPcts, err := cpu.Percent(0, false)
	if err == nil && len(cpuPcts) > 0 {
		p.CPUIdlePct = cpuPcts[0] // Simplified
	}
	
	cpuTimes, err := cpu.Times(false)
	if err == nil && len(cpuTimes) > 0 {
		t := cpuTimes[0]
		total := t.User + t.System + t.Idle + t.Iowait + t.Nice + t.Irq + t.Softirq + t.Steal
		p.CPUUserPct = (t.User / total) * 100
		p.CPUSystemPct = (t.System / total) * 100
		p.CPUIOWaitPct = (t.Iowait / total) * 100
	}
	
	p.CPUCores, _ = cpu.Counts(true)

	l, err := load.Avg()
	if err == nil {
		p.Load1m = l.Load1
		p.Load5m = l.Load5
		p.Load15m = l.Load15
	}

	// Postgres processes
	procs, err := process.Processes()
	if err == nil {
		for _, pr := range procs {
			name, _ := pr.Name()
			if name == "postgres" {
				m, _ := pr.MemoryInfo()
				if m != nil {
					p.PostgresRSS += m.RSS
					p.PostgresVSZ += m.VMS
				}
				p.BackendCount++
			}
		}
	}

	return p, nil
}

func sendMetrics(url, key string, payload *OSMetricsPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
