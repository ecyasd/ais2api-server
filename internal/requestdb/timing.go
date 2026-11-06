package requestdb

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Mag1cFall/AIStudio2API/internal/telemetry"
)

const timingSchema = `
CREATE TABLE IF NOT EXISTS server_request_timings (
 id TEXT PRIMARY KEY, time INTEGER NOT NULL, revision INTEGER NOT NULL DEFAULT 0, report TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS server_request_timings_time ON server_request_timings(time);`

type timingRow struct {
	id     string
	report telemetry.Report
}

// RecordTiming uses the same bounded, drained writer as the upstream ledger.
// Late background cleanup updates do not hold up the client completion marker.
func (store *Store) RecordTiming(id string, report telemetry.Report) {
	if id != "" {
		store.enqueue(write{timing: &timingRow{id: id, report: report}})
	}
}

func (store *Store) RequestTiming(ctx context.Context, id string) (telemetry.Report, error) {
	var report telemetry.Report
	var raw string
	err := store.db.QueryRowContext(ctx, `SELECT report FROM server_request_timings WHERE id=?`, id).Scan(&raw)
	if err != nil {
		return report, err
	}
	return report, json.Unmarshal([]byte(raw), &report)
}

func timingCutoff() int64 { return time.Now().Add(-7 * 24 * time.Hour).UnixMilli() }
