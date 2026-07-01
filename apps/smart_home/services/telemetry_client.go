package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

// TelemetryService stores telemetry records in the telemetry-service
type TelemetryService struct {
	BaseURL    string
	HTTPClient *http.Client

	// Enabled controls whether calls are actually made
	Enabled bool
}

// TelemetryCreateRequest mirrors the telemetry-service POST /telemetry body
type TelemetryCreateRequest struct {
	LegacySensorID string    `json:"legacy_sensor_id"`
	Metric         string    `json:"metric"`
	Value          float64   `json:"value"`
	Unit           string    `json:"unit"`
	Location       string    `json:"location"`
	ObservedAt     time.Time `json:"observed_at"`
}

// TelemetryRecord mirrors the telemetry-service response model
type TelemetryRecord struct {
	ID             string    `json:"id"`
	LegacySensorID string    `json:"legacy_sensor_id"`
	DeviceID       *string   `json:"device_id,omitempty"`
	Metric         string    `json:"metric"`
	Value          float64   `json:"value"`
	Unit           string    `json:"unit"`
	Location       string    `json:"location"`
	ObservedAt     time.Time `json:"observed_at"`
	CreatedAt      time.Time `json:"created_at"`
}

// NewTelemetryService creates a TelemetryService
func NewTelemetryService(baseURL string, enabled bool) *TelemetryService {
	return &TelemetryService{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		Enabled: enabled,
	}
}

// RecordTemperature sends a temperature measurement to the telemetry service
func (c *TelemetryService) RecordTemperature(legacySensorID string, value float64, unit, location string, observedAt time.Time) {
	if !c.Enabled {
		return
	}

	if unit == "" {
		unit = "Celsius"
	}
	req := TelemetryCreateRequest{
		LegacySensorID: legacySensorID,
		Metric:         "temperature",
		Value:          value,
		Unit:           unit,
		Location:       location,
		ObservedAt:     observedAt,
	}
	log.Printf("[telemetry-client] RecordTemperature sensor=%s POST %s/telemetry", legacySensorID, c.BaseURL)
	if err := c.postTelemetry(req); err != nil {
		log.Printf("[telemetry-client] RecordTemperature sensor=%s: %v", legacySensorID, err)
	}
}

// GetLatestTelemetry calls GET /telemetry/latest?legacy_sensor_id={id}
func (c *TelemetryService) GetLatestTelemetry(legacySensorID string) (*TelemetryRecord, error) {
	if !c.Enabled {
		return nil, fmt.Errorf("telemetry client is disabled")
	}

	path := fmt.Sprintf("/telemetry/latest?legacy_sensor_id=%s", url.QueryEscape(legacySensorID))
	log.Printf("[telemetry-client] GetLatestTelemetry sensor=%s GET %s%s", legacySensorID, c.BaseURL, path)

	var record TelemetryRecord
	if err := c.getJSON(path, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// ListTelemetry calls GET /telemetry?legacy_sensor_id={id}&metric={metric}
func (c *TelemetryService) ListTelemetry(legacySensorID, metric string) ([]TelemetryRecord, error) {
	if !c.Enabled {
		return nil, fmt.Errorf("telemetry client is disabled")
	}

	q := url.Values{}
	q.Set("legacy_sensor_id", legacySensorID)
	if metric != "" {
		q.Set("metric", metric)
	}
	path := "/telemetry?" + q.Encode()
	log.Printf("[telemetry-client] ListTelemetry sensor=%s GET %s%s", legacySensorID, c.BaseURL, path)

	var records []TelemetryRecord
	if err := c.getJSON(path, &records); err != nil {
		return nil, err
	}
	if records == nil {
		records = []TelemetryRecord{}
	}
	return records, nil
}

func (c *TelemetryService) postTelemetry(body TelemetryCreateRequest) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/telemetry", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (c *TelemetryService) getJSON(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
