package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSignalHistoryRecordAndRetention(t *testing.T) {
	if err := Init(filepath.Join(t.TempDir(), "signal.db")); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 20, 12, 1, 0, 0, time.UTC)
	if err := RecordSignalHistory("DJI", now, SignalValues{RSSI: -80, RSRP: -100}); err != nil {
		t.Fatal(err)
	}
	if err := RecordSignalHistory("DJI", now.Add(20*time.Second), SignalValues{RSRQ: -12, SINR: 18}); err != nil {
		t.Fatal(err)
	}
	if err := RecordSignalHistory("DJI", now.Add(time.Minute), SignalValues{RSSI: -90, RSRP: -110}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := DB.Model(&SignalHistory{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("rows=%d want=2", count)
	}
	points, err := GetSignalHistory("DJI", now.Add(-time.Minute), now.Add(10*time.Minute), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0].SampleCount != 2 || points[0].RSSI == nil || *points[0].RSSI != -85 {
		t.Fatalf("unexpected points: %+v", points)
	}
	if err := RecordSignalHistory("old", now.AddDate(0, 0, -8), SignalValues{RSSI: -100}); err != nil {
		t.Fatal(err)
	}
	if err := SetSignalHistoryRetentionDays(7, now); err != nil {
		t.Fatal(err)
	}
	if err := DB.Model(&SignalHistory{}).Where("device_id = ?", "old").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expired rows=%d", count)
	}
}

func TestValidateSignalHistoryRetentionDays(t *testing.T) {
	if ValidateSignalHistoryRetentionDays(0) == nil || ValidateSignalHistoryRetentionDays(3651) == nil {
		t.Fatal("invalid retention accepted")
	}
	if err := ValidateSignalHistoryRetentionDays(30); err != nil {
		t.Fatal(err)
	}
}
