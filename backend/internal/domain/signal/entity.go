package signal

import (
	"time"

	"github.com/google/uuid"
)

type Signal struct {
	ServerID    uuid.UUID `json:"server_id"`
	SignalKey   string    `json:"signal_key"`
	SignalValue float64   `json:"signal_value"`
	CollectedAt time.Time `json:"capture_timestamp"`
}

type SignalSnapshot struct {
	ID        int64                  `json:"snapshot_id"`
	ServerID  uuid.UUID              `json:"server_id"`
	DBType    string                 `json:"db_type"`
	Snapshot  map[string]interface{} `json:"snapshot"`
	CreatedAt time.Time              `json:"created_at"`
}
