package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// DeviceService sends sensor-lifecycle events to the device-service
type DeviceService struct {
	BaseURL    string
	HTTPClient *http.Client

	// Enabled controls whether calls are actually made
	Enabled bool
}

// DeviceUpsertRequest mirrors the device-service request body
type DeviceUpsertRequest struct {
	LegacySensorID string   `json:"legacy_sensor_id"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Location       string   `json:"location"`
	Unit           string   `json:"unit"`
	Status         string   `json:"status"`
	Capabilities   []string `json:"capabilities"`
}

// DeviceResponse mirrors the device-service response model
type DeviceResponse struct {
	ID             string    `json:"id"`
	LegacySensorID string    `json:"legacy_sensor_id"`
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	Location       string    `json:"location"`
	Unit           string    `json:"unit"`
	Status         string    `json:"status"`
	Capabilities   []string  `json:"capabilities"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// NewDeviceService creates a DeviceService
func NewDeviceService(baseURL string, enabled bool) *DeviceService {
	return &DeviceService{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		Enabled: enabled,
	}
}

// CreateDevice calls POST /devices
func (c *DeviceService) CreateDevice(legacySensorID, name, sensorType, location, unit, status string) {
	if !c.Enabled {
		return
	}

	req := DeviceUpsertRequest{
		LegacySensorID: legacySensorID,
		Name:           name,
		Type:           sensorType,
		Location:       location,
		Unit:           unit,
		Status:         status,
	}
	log.Printf("[device-client] CreateDevice sensor=%s POST %s/devices", legacySensorID, c.BaseURL)
	if err := c.post("/devices", req); err != nil {
		log.Printf("[device-client] CreateDevice sensor=%s: %v", legacySensorID, err)
	}
}

// UpdateDevice calls PUT /devices/by-legacy-sensor/{id}
func (c *DeviceService) UpdateDevice(legacySensorID, name, sensorType, location, unit, status string) {
	if !c.Enabled {
		return
	}

	req := DeviceUpsertRequest{
		LegacySensorID: legacySensorID,
		Name:           name,
		Type:           sensorType,
		Location:       location,
		Unit:           unit,
		Status:         status,
	}
	url := fmt.Sprintf("%s/devices/by-legacy-sensor/%s", c.BaseURL, legacySensorID)
	log.Printf("[device-client] UpdateDevice sensor=%s PUT %s", legacySensorID, url)
	if err := c.putJSON(url, req); err != nil {
		log.Printf("[device-client] UpdateDevice sensor=%s: %v", legacySensorID, err)
	}
}

// DeleteDevice calls DELETE /devices/by-legacy-sensor/{id}
func (c *DeviceService) DeleteDevice(legacySensorID string) {
	if !c.Enabled {
		return
	}
	url := fmt.Sprintf("%s/devices/by-legacy-sensor/%s", c.BaseURL, legacySensorID)
	log.Printf("[device-client] DeleteDevice sensor=%s DELETE %s", legacySensorID, url)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		log.Printf("[device-client] DeleteDevice build request sensor=%s: %v", legacySensorID, err)
		return
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		log.Printf("[device-client] DeleteDevice sensor=%s: %v", legacySensorID, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("[device-client] DeleteDevice sensor=%s: unexpected status %d", legacySensorID, resp.StatusCode)
	}
}

// GetDeviceByLegacySensorID calls GET /devices/by-legacy-sensor/{id}
func (c *DeviceService) GetDeviceByLegacySensorID(legacySensorID string) (*DeviceResponse, error) {
	if !c.Enabled {
		return nil, fmt.Errorf("device client is disabled")
	}
	url := fmt.Sprintf("%s/devices/by-legacy-sensor/%s", c.BaseURL, legacySensorID)
	log.Printf("[device-client] GetDeviceByLegacySensorID sensor=%s GET %s", legacySensorID, url)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var device DeviceResponse
	if err := json.NewDecoder(resp.Body).Decode(&device); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &device, nil
}

// post sends a JSON POST request to baseURL+path
func (c *DeviceService) post(path string, body any) error {
	return c.sendJSON(http.MethodPost, c.BaseURL+path, body)
}

// putJSON sends a JSON PUT request to the given full URL
func (c *DeviceService) putJSON(url string, body any) error {
	return c.sendJSON(http.MethodPut, url, body)
}

func (c *DeviceService) sendJSON(method, url string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
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
