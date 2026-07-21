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
	if err := RecordSignalHistory("DJI", "iccid-a", now, SignalValues{RSSI: -80, RSRP: -100}); err != nil {
		t.Fatal(err)
	}
	if err := RecordSignalHistory("DJI", "iccid-a", now.Add(20*time.Second), SignalValues{RSRQ: -12, SINR: 18}); err != nil {
		t.Fatal(err)
	}
	if err := RecordSignalHistory("DJI", "iccid-a", now.Add(time.Minute), SignalValues{RSSI: -90, RSRP: -110}); err != nil {
		t.Fatal(err)
	}
	// 同一设备、同一分钟切换到新 Profile 时必须形成独立记录。
	if err := RecordSignalHistory("DJI", "iccid-b", now.Add(time.Minute), SignalValues{RSSI: -60}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := DB.Model(&SignalHistory{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("rows=%d want=3", count)
	}
	points, err := GetSignalHistory("DJI", "iccid-a", now.Add(-time.Minute), now.Add(10*time.Minute), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0].SampleCount != 2 || points[0].RSSI == nil || *points[0].RSSI != -85 {
		t.Fatalf("unexpected points: %+v", points)
	}
	profileB, err := GetSignalHistory("DJI", "iccid-b", now.Add(-time.Minute), now.Add(10*time.Minute), 5*time.Minute)
	if err != nil || len(profileB) != 1 || profileB[0].RSSI == nil || *profileB[0].RSSI != -60 {
		t.Fatalf("profile B mixed or missing: points=%+v err=%v", profileB, err)
	}
	if err := RecordSignalHistory("old", "old-iccid", now.AddDate(0, 0, -8), SignalValues{RSSI: -100}); err != nil {
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

func TestSignalHistoryMigratesDeviceOnlySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-signal.db")
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	if err := DB.Migrator().DropTable(&SignalHistory{}); err != nil {
		t.Fatal(err)
	}
	if err := DB.Exec(`CREATE TABLE signal_history (
		id integer PRIMARY KEY AUTOINCREMENT,
		device_id text NOT NULL,
		recorded_at datetime NOT NULL,
		rssi integer, rsrp integer, rsrq integer, sinr integer, nr5g_sinr integer,
		created_at datetime, updated_at datetime
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Exec("CREATE UNIQUE INDEX uk_signal_history_device_time ON signal_history(device_id, recorded_at)").Error; err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	if err := DB.Exec("INSERT INTO signal_history(device_id, recorded_at, rssi) VALUES(?, ?, ?)", "DJI", now, -100).Error; err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	if !DB.Migrator().HasColumn(&SignalHistory{}, "ICCID") {
		t.Fatal("ICCID column was not added")
	}
	if DB.Migrator().HasIndex(&SignalHistory{}, "uk_signal_history_device_time") {
		t.Fatal("legacy device-only unique index was not removed")
	}
	if err := RecordSignalHistory("DJI", "iccid-a", now, SignalValues{RSSI: -80}); err != nil {
		t.Fatal(err)
	}
	if err := RecordSignalHistory("DJI", "iccid-b", now, SignalValues{RSSI: -60}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := DB.Model(&SignalHistory{}).Where("device_id = ? AND recorded_at = ?", "DJI", now).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("same minute rows after migration=%d want=3", count)
	}
}
