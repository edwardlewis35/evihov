package device

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/boa-z/vohive/internal/db"
	"github.com/boa-z/vohive/internal/modem"
)

func TestWorkerPersistsSignalHistoryOncePerMinute(t *testing.T) {
	if err := db.Init(filepath.Join(t.TempDir(), "signal.db")); err != nil {
		t.Fatal(err)
	}
	w := &Worker{ID: "DJI"}
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	if err := w.persistSignalHistory(modem.DeviceStatus{SignalDBM: -80}, now); err != nil {
		t.Fatal(err)
	}
	if err := w.persistSignalHistory(modem.DeviceStatus{SignalDBM: -90}, now.Add(20*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := w.persistSignalHistory(modem.DeviceStatus{SignalDBM: -90}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var rows []db.SignalHistory
	if err := db.DB.Where("device_id = ?", "DJI").Order("recorded_at").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].RSSI == nil || *rows[0].RSSI != -80 {
		t.Fatalf("rows=%+v", rows)
	}
}
