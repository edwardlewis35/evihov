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
	if err := w.persistSignalHistory(modem.DeviceStatus{SignalDBM: -80}, "iccid-a", now); err != nil {
		t.Fatal(err)
	}
	if err := w.persistSignalHistory(modem.DeviceStatus{SignalDBM: -90}, "iccid-a", now.Add(20*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := w.persistSignalHistory(modem.DeviceStatus{SignalDBM: -60}, "iccid-b", now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := w.persistSignalHistory(modem.DeviceStatus{SignalDBM: -90}, "iccid-b", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var rows []db.SignalHistory
	if err := db.DB.Where("device_id = ?", "DJI").Order("recorded_at, iccid").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].ICCID != "iccid-a" || rows[1].ICCID != "iccid-b" {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestConfirmedICCIDSuppressesESIMTransitionTarget(t *testing.T) {
	w := &Worker{ID: "DJI"}
	w.state.Identity.ICCID = "iccid-a"
	w.state.Identity.Ready = true
	w.state.Identity.Phase = simIdentityPhaseReady
	if got := w.ConfirmedICCID(); got != "iccid-a" {
		t.Fatalf("ConfirmedICCID()=%q", got)
	}
	w.BeginSIMIdentityTransition("iccid-b", "test")
	if got := w.ConfirmedICCID(); got != "" {
		t.Fatalf("transition target leaked as confirmed ICCID: %q", got)
	}
	if got := w.CurrentICCID(); got != "iccid-b" {
		t.Fatalf("CurrentICCID()=%q want target", got)
	}
}
