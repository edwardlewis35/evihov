package api

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/boa-z/vohive/internal/db"
	"github.com/gin-gonic/gin"
)

type signalHistoryResponse struct {
	DeviceID      string                  `json:"device_id"`
	ICCID         string                  `json:"iccid"`
	IdentityReady bool                    `json:"identity_ready"`
	Range         string                  `json:"range"`
	RetentionDays int                     `json:"retention_days"`
	Since         time.Time               `json:"since"`
	Until         time.Time               `json:"until"`
	BucketSeconds int64                   `json:"bucket_seconds"`
	Points        []db.SignalHistoryPoint `json:"points"`
}

type signalHistorySettingResponse struct {
	RetentionDays int `json:"retention_days"`
	MinDays       int `json:"min_days"`
	MaxDays       int `json:"max_days"`
}

func (s *Server) handleGetDeviceSignalHistory(c *gin.Context) {
	deviceID := strings.TrimSpace(deviceIDParam(c))
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "device_id 不能为空"})
		return
	}
	days, err := db.GetSignalHistoryRetentionDays()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	now := time.Now().UTC()
	rangeName, since, bucket, err := resolveSignalHistoryRange(c.DefaultQuery("range", "day"), now, days)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	iccid := s.confirmedSignalHistoryICCID(deviceID)
	if iccid == "" {
		c.JSON(http.StatusOK, signalHistoryResponse{
			DeviceID: deviceID, ICCID: "", IdentityReady: false,
			Range: rangeName, RetentionDays: days, Since: since, Until: now,
			BucketSeconds: int64(bucket / time.Second), Points: make([]db.SignalHistoryPoint, 0),
		})
		return
	}
	points, err := db.GetSignalHistory(deviceID, iccid, since, now, bucket)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, signalHistoryResponse{
		DeviceID: deviceID, ICCID: iccid, IdentityReady: true, Range: rangeName, RetentionDays: days,
		Since: since, Until: now, BucketSeconds: int64(bucket / time.Second), Points: points,
	})
}

func (s *Server) confirmedSignalHistoryICCID(deviceID string) string {
	if s != nil && s.pool != nil {
		if worker := s.pool.GetWorker(deviceID); worker != nil {
			return strings.TrimSpace(worker.ConfirmedICCID())
		}
	}
	return strings.TrimSpace(db.CurrentICCIDForDevice(deviceID))
}

func (s *Server) handleGetSignalHistorySetting(c *gin.Context) {
	days, err := db.GetSignalHistoryRetentionDays()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, newSignalHistorySettingResponse(days))
}

func (s *Server) handleUpdateSignalHistorySetting(c *gin.Context) {
	var req struct {
		RetentionDays int `json:"retention_days" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "retention_days 必须是整数"})
		return
	}
	if err := db.SetSignalHistoryRetentionDays(req.RetentionDays, time.Now()); err != nil {
		status := http.StatusInternalServerError
		if db.ValidateSignalHistoryRetentionDays(req.RetentionDays) != nil {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"status": "error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, newSignalHistorySettingResponse(req.RetentionDays))
}

func newSignalHistorySettingResponse(days int) signalHistorySettingResponse {
	return signalHistorySettingResponse{days, db.MinSignalHistoryRetentionDays, db.MaxSignalHistoryRetentionDays}
}

func resolveSignalHistoryRange(value string, now time.Time, retentionDays int) (string, time.Time, time.Duration, error) {
	rangeName := strings.ToLower(strings.TrimSpace(value))
	switch rangeName {
	case "day":
		return rangeName, now.Add(-24 * time.Hour), 5 * time.Minute, nil
	case "week":
		return rangeName, now.Add(-7 * 24 * time.Hour), 30 * time.Minute, nil
	case "month":
		return rangeName, now.Add(-30 * 24 * time.Hour), 2 * time.Hour, nil
	case "retention":
		if err := db.ValidateSignalHistoryRetentionDays(retentionDays); err != nil {
			return "", time.Time{}, 0, err
		}
		span := time.Duration(retentionDays) * 24 * time.Hour
		minutes := int64(math.Ceil(float64(span) / float64(720*time.Minute)))
		if minutes < 1 {
			minutes = 1
		}
		return rangeName, now.Add(-span), time.Duration(minutes) * time.Minute, nil
	default:
		return "", time.Time{}, 0, fmt.Errorf("range 必须是 day、week、month 或 retention")
	}
}
