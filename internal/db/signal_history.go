package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	DefaultSignalHistoryRetentionDays = 30
	MinSignalHistoryRetentionDays     = 1
	MaxSignalHistoryRetentionDays     = 3650
	signalHistorySettingID            = 1
)

type SignalHistory struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	DeviceID   string    `gorm:"not null;size:128;uniqueIndex:uk_signal_history_device_iccid_time,priority:1;index:idx_signal_history_device_iccid_time,priority:1" json:"device_id"`
	ICCID      string    `gorm:"column:iccid;not null;default:'';size:64;uniqueIndex:uk_signal_history_device_iccid_time,priority:2;index:idx_signal_history_device_iccid_time,priority:2" json:"iccid"`
	RecordedAt time.Time `gorm:"not null;uniqueIndex:uk_signal_history_device_iccid_time,priority:3;index:idx_signal_history_device_iccid_time,priority:3" json:"recorded_at"`
	RSSI       *int      `gorm:"column:rssi" json:"rssi,omitempty"`
	RSRP       *int      `gorm:"column:rsrp" json:"rsrp,omitempty"`
	RSRQ       *int      `gorm:"column:rsrq" json:"rsrq,omitempty"`
	SINR       *int      `gorm:"column:sinr" json:"sinr,omitempty"`
	NR5GSINR   *int      `gorm:"column:nr5g_sinr" json:"nr5g_sinr,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (SignalHistory) TableName() string { return "signal_history" }

type SignalHistorySetting struct {
	ID            uint      `gorm:"primaryKey;autoIncrement:false" json:"-"`
	RetentionDays int       `gorm:"not null;default:30" json:"retention_days"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (SignalHistorySetting) TableName() string { return "signal_history_settings" }

// migrateSignalHistoryICCIDSchema removes the device-only uniqueness left by
// the first version of signal history. Rows without ICCID stay unassigned and
// are deliberately excluded from per-profile queries.
func migrateSignalHistoryICCIDSchema(tx *gorm.DB) error {
	const legacyIndex = "uk_signal_history_device_time"
	if tx.Migrator().HasIndex(&SignalHistory{}, legacyIndex) {
		return tx.Migrator().DropIndex(&SignalHistory{}, legacyIndex)
	}
	return nil
}

type SignalValues struct {
	RSSI, RSRP, RSRQ, SINR, NR5GSINR int
}

type SignalHistoryPoint struct {
	RecordedAt  time.Time `json:"recorded_at"`
	RSSI        *float64  `json:"rssi"`
	RSRP        *float64  `json:"rsrp"`
	RSRQ        *float64  `json:"rsrq"`
	SINR        *float64  `json:"sinr"`
	NR5GSINR    *float64  `json:"nr5g_sinr"`
	SampleCount int64     `json:"sample_count"`
}

func ValidateSignalHistoryRetentionDays(days int) error {
	if days < MinSignalHistoryRetentionDays || days > MaxSignalHistoryRetentionDays {
		return fmt.Errorf("retention_days must be between %d and %d", MinSignalHistoryRetentionDays, MaxSignalHistoryRetentionDays)
	}
	return nil
}

func ensureSignalHistorySetting(tx *gorm.DB) error {
	setting := SignalHistorySetting{ID: signalHistorySettingID, RetentionDays: DefaultSignalHistoryRetentionDays}
	return tx.Where("id = ?", signalHistorySettingID).FirstOrCreate(&setting).Error
}

func GetSignalHistoryRetentionDays() (int, error) {
	if DB == nil {
		return 0, fmt.Errorf("db not initialized")
	}
	if err := ensureSignalHistorySetting(DB); err != nil {
		return 0, err
	}
	var setting SignalHistorySetting
	if err := DB.First(&setting, signalHistorySettingID).Error; err != nil {
		return 0, err
	}
	if err := ValidateSignalHistoryRetentionDays(setting.RetentionDays); err != nil {
		return 0, err
	}
	return setting.RetentionDays, nil
}

func SetSignalHistoryRetentionDays(days int, now time.Time) error {
	if err := ValidateSignalHistoryRetentionDays(days); err != nil {
		return err
	}
	if DB == nil {
		return fmt.Errorf("db not initialized")
	}
	if now.IsZero() {
		now = time.Now()
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		setting := SignalHistorySetting{ID: signalHistorySettingID, RetentionDays: days}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"retention_days", "updated_at"}),
		}).Create(&setting).Error; err != nil {
			return err
		}
		return deleteExpiredSignalHistory(tx, now, days)
	})
}

func CleanupSignalHistory(now time.Time) error {
	if DB == nil {
		return fmt.Errorf("db not initialized")
	}
	days, err := GetSignalHistoryRetentionDays()
	if err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now()
	}
	return deleteExpiredSignalHistory(DB, now, days)
}

func deleteExpiredSignalHistory(tx *gorm.DB, now time.Time, days int) error {
	if err := ValidateSignalHistoryRetentionDays(days); err != nil {
		return err
	}
	return tx.Where("recorded_at < ?", now.UTC().AddDate(0, 0, -days)).Delete(&SignalHistory{}).Error
}

func DeleteSignalHistoryForDevice(deviceID string) error {
	if DB == nil {
		return fmt.Errorf("db not initialized")
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return fmt.Errorf("device_id is required")
	}
	return DB.Where("device_id = ?", deviceID).Delete(&SignalHistory{}).Error
}

func RecordSignalHistory(deviceID, iccid string, recordedAt time.Time, values SignalValues) error {
	if DB == nil {
		return fmt.Errorf("db not initialized")
	}
	deviceID = strings.TrimSpace(deviceID)
	iccid = strings.TrimSpace(iccid)
	if deviceID == "" || iccid == "" {
		return fmt.Errorf("device_id and iccid are required")
	}
	if recordedAt.IsZero() {
		recordedAt = time.Now()
	}
	row := SignalHistory{
		DeviceID: deviceID, ICCID: iccid, RecordedAt: recordedAt.UTC().Truncate(time.Minute),
		RSSI: validSignalValue(values.RSSI), RSRP: validSignalValue(values.RSRP),
		RSRQ: validSignalValue(values.RSRQ), SINR: validSignalValue(values.SINR),
		NR5GSINR: validSignalValue(values.NR5GSINR),
	}
	if row.RSSI == nil && row.RSRP == nil && row.RSRQ == nil && row.SINR == nil && row.NR5GSINR == nil {
		return nil
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "device_id"}, {Name: "iccid"}, {Name: "recorded_at"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"rssi":       gorm.Expr("COALESCE(excluded.rssi, signal_history.rssi)"),
			"rsrp":       gorm.Expr("COALESCE(excluded.rsrp, signal_history.rsrp)"),
			"rsrq":       gorm.Expr("COALESCE(excluded.rsrq, signal_history.rsrq)"),
			"sinr":       gorm.Expr("COALESCE(excluded.sinr, signal_history.sinr)"),
			"nr5g_sinr":  gorm.Expr("COALESCE(excluded.nr5g_sinr, signal_history.nr5g_sinr)"),
			"updated_at": time.Now().UTC(),
		}),
	}).Create(&row).Error
}

func validSignalValue(value int) *int {
	if value == 0 || value == -999 {
		return nil
	}
	v := value
	return &v
}

type signalHistoryAggregateRow struct {
	BucketUnix                       int64
	RSSI, RSRP, RSRQ, SINR, NR5GSINR sql.NullFloat64
	SampleCount                      int64
}

func GetSignalHistory(deviceID, iccid string, since, until time.Time, bucket time.Duration) ([]SignalHistoryPoint, error) {
	if DB == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	deviceID = strings.TrimSpace(deviceID)
	iccid = strings.TrimSpace(iccid)
	if deviceID == "" || iccid == "" || since.IsZero() || until.IsZero() || !since.Before(until) || bucket < time.Minute {
		return nil, fmt.Errorf("invalid signal history query")
	}
	seconds := int64(bucket / time.Second)
	rows := make([]signalHistoryAggregateRow, 0)
	query := `SELECT
		(CAST(strftime('%s', recorded_at) AS INTEGER) / ?) * ? AS bucket_unix,
		AVG(rssi) AS rssi, AVG(rsrp) AS rsrp, AVG(rsrq) AS rsrq,
		AVG(sinr) AS sinr, AVG(nr5g_sinr) AS nr5g_sinr, COUNT(*) AS sample_count
		FROM signal_history
		WHERE device_id = ? AND iccid = ? AND recorded_at >= ? AND recorded_at <= ?
		GROUP BY bucket_unix HAVING bucket_unix IS NOT NULL ORDER BY bucket_unix ASC`
	if err := DB.Raw(query, seconds, seconds, deviceID, iccid, since.UTC(), until.UTC()).Scan(&rows).Error; err != nil {
		return nil, err
	}
	points := make([]SignalHistoryPoint, 0, len(rows))
	for _, row := range rows {
		points = append(points, SignalHistoryPoint{
			RecordedAt: time.Unix(row.BucketUnix, 0).UTC(), RSSI: nullFloat64Ptr(row.RSSI),
			RSRP: nullFloat64Ptr(row.RSRP), RSRQ: nullFloat64Ptr(row.RSRQ),
			SINR: nullFloat64Ptr(row.SINR), NR5GSINR: nullFloat64Ptr(row.NR5GSINR),
			SampleCount: row.SampleCount,
		})
	}
	return points, nil
}

func nullFloat64Ptr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	v := value.Float64
	return &v
}
