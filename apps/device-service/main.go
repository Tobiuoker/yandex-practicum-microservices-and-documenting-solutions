package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Device is the canonical domain model stored in the registry
type Device struct {
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

// DeviceUpsertRequest is the request body for POST /devices and PUT /devices/by-legacy-sensor/{id}
type DeviceUpsertRequest struct {
	LegacySensorID string   `json:"legacy_sensor_id"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Location       string   `json:"location"`
	Unit           string   `json:"unit"`
	Status         string   `json:"status"`
	Capabilities   []string `json:"capabilities"`
}

type server struct {
	pool *pgxpool.Pool
}

func main() {
	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/devices")
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("unable to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("unable to ping database: %v", err)
	}
	log.Println("connected to database")

	s := &server{pool: pool}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /devices", s.handleCreateDevice)
	mux.HandleFunc("GET /devices", s.handleListDevices)
	mux.HandleFunc("GET /devices/by-legacy-sensor/{sensorId}", s.handleGetDeviceByLegacySensor)
	mux.HandleFunc("PUT /devices/by-legacy-sensor/{sensorId}", s.handleUpdateDeviceByLegacySensor)
	mux.HandleFunc("DELETE /devices/by-legacy-sensor/{sensorId}", s.handleDeleteDeviceByLegacySensor)

	port := getEnv("PORT", ":8082")
	log.Printf("device-service listening on %s", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleCreateDevice upserts a device keyed by legacy_sensor_id
func (s *server) handleCreateDevice(w http.ResponseWriter, r *http.Request) {
	var req DeviceUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid JSON: "+err.Error()))
		return
	}
	if req.LegacySensorID == "" {
		writeJSON(w, http.StatusBadRequest, errResp("legacy_sensor_id is required"))
		return
	}

	caps := req.Capabilities
	if caps == nil {
		caps = capabilitiesForType(req.Type)
	}

	if req.Status == "" {
		req.Status = "inactive"
	}

	device, err := s.upsertDevice(r.Context(), req, caps)
	if err != nil {
		log.Printf("upsertDevice error: %v", err)
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, device)
}

func (s *server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	typ := q.Get("type")
	status := q.Get("status")
	location := q.Get("location")

	devices, err := s.listDevices(r.Context(), typ, status, location)
	if err != nil {
		log.Printf("listDevices error: %v", err)
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	if devices == nil {
		devices = []Device{}
	}
	writeJSON(w, http.StatusOK, devices)
}

// handleGetDeviceByLegacySensor returns a single device by its legacy sensor ID
func (s *server) handleGetDeviceByLegacySensor(w http.ResponseWriter, r *http.Request) {
	sensorID := r.PathValue("sensorId")
	device, err := s.getDeviceByLegacySensorID(r.Context(), sensorID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, errResp("device not found"))
			return
		}
		log.Printf("getDeviceByLegacySensorID error: %v", err)
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, device)
}

// handleUpdateDeviceByLegacySensor updates a device identified by legacy sensor ID
func (s *server) handleUpdateDeviceByLegacySensor(w http.ResponseWriter, r *http.Request) {
	sensorID := r.PathValue("sensorId")
	var req DeviceUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid JSON: "+err.Error()))
		return
	}

	req.LegacySensorID = sensorID

	caps := req.Capabilities
	if caps == nil && req.Type != "" {
		caps = capabilitiesForType(req.Type)
	}

	device, err := s.updateDeviceByLegacySensorID(r.Context(), sensorID, req, caps)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, errResp("device not found"))
			return
		}
		log.Printf("updateDeviceByLegacySensorID error: %v", err)
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, device)
}

// handleDeleteDeviceByLegacySensor marks a device as deleted/inactive
func (s *server) handleDeleteDeviceByLegacySensor(w http.ResponseWriter, r *http.Request) {
	sensorID := r.PathValue("sensorId")
	if err := s.markDeviceDeleted(r.Context(), sensorID); err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, errResp("device not found"))
			return
		}
		log.Printf("markDeviceDeleted error: %v", err)
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"legacy_sensor_id": sensorID,
		"status":           "deleted",
		"deleted":          true,
	})
}

func (s *server) upsertDevice(ctx context.Context, req DeviceUpsertRequest, caps []string) (Device, error) {
	query := `
		INSERT INTO devices (legacy_sensor_id, name, type, location, unit, status, capabilities, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		ON CONFLICT (legacy_sensor_id) DO UPDATE
		  SET name         = EXCLUDED.name,
		      type         = EXCLUDED.type,
		      location     = EXCLUDED.location,
		      unit         = EXCLUDED.unit,
		      status       = EXCLUDED.status,
		      capabilities = EXCLUDED.capabilities,
		      updated_at   = NOW()
		RETURNING id, legacy_sensor_id, name, type, location, unit, status, capabilities, created_at, updated_at
	`
	return s.scanDevice(ctx, query,
		req.LegacySensorID, req.Name, req.Type, req.Location, req.Unit, req.Status, caps,
	)
}

func (s *server) listDevices(ctx context.Context, typ, status, location string) ([]Device, error) {
	var conditions []string
	var args []any

	if typ != "" {
		args = append(args, typ)
		conditions = append(conditions, fmt.Sprintf("type = $%d", len(args)))
	}

	if status != "" {
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	} else {
		conditions = append(conditions, "status <> 'deleted'")
	}
	if location != "" {
		args = append(args, location)
		conditions = append(conditions, fmt.Sprintf("location = $%d", len(args)))
	}

	query := `
		SELECT id, legacy_sensor_id, name, type, location, unit, status, capabilities, created_at, updated_at
		FROM devices`
	if len(conditions) > 0 {
		query += "\n\t\tWHERE " + strings.Join(conditions, " AND ")
	}
	query += "\n\t\tORDER BY created_at"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query devices: %w", err)
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		d, err := scanDeviceRow(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (s *server) getDeviceByLegacySensorID(ctx context.Context, sensorID string) (Device, error) {
	query := `
		SELECT id, legacy_sensor_id, name, type, location, unit, status, capabilities, created_at, updated_at
		FROM devices
		WHERE legacy_sensor_id = $1
	`
	return s.scanDevice(ctx, query, sensorID)
}

func (s *server) updateDeviceByLegacySensorID(ctx context.Context, sensorID string, req DeviceUpsertRequest, caps []string) (Device, error) {
	query := `
		UPDATE devices
		SET name         = COALESCE(NULLIF($2, ''), name),
		    type         = COALESCE(NULLIF($3, ''), type),
		    location     = COALESCE(NULLIF($4, ''), location),
		    unit         = COALESCE(NULLIF($5, ''), unit),
		    status       = COALESCE(NULLIF($6, ''), status),
		    capabilities = CASE WHEN $7::text[] IS NOT NULL THEN $7 ELSE capabilities END,
		    updated_at   = NOW()
		WHERE legacy_sensor_id = $1
		RETURNING id, legacy_sensor_id, name, type, location, unit, status, capabilities, created_at, updated_at
	`
	return s.scanDevice(ctx, query, sensorID, req.Name, req.Type, req.Location, req.Unit, req.Status, caps)
}

func (s *server) markDeviceDeleted(ctx context.Context, sensorID string) error {
	query := `
		UPDATE devices
		SET status = 'deleted', updated_at = NOW()
		WHERE legacy_sensor_id = $1
	`
	result, err := s.pool.Exec(ctx, query, sensorID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *server) scanDevice(ctx context.Context, query string, args ...any) (Device, error) {
	row := s.pool.QueryRow(ctx, query, args...)
	return scanDeviceRow(row)
}

func scanDeviceRow(row interface {
	Scan(dest ...any) error
}) (Device, error) {
	var d Device
	var caps []string
	err := row.Scan(
		&d.ID,
		&d.LegacySensorID,
		&d.Name,
		&d.Type,
		&d.Location,
		&d.Unit,
		&d.Status,
		&caps,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		return Device{}, err
	}
	if caps == nil {
		caps = []string{}
	}
	d.Capabilities = caps
	return d, nil
}

func capabilitiesForType(sensorType string) []string {
	switch sensorType {
	case "temperature":
		return []string{"temperature_sensor"}
	case "humidity":
		return []string{"humidity_sensor"}
	case "motion":
		return []string{"motion_sensor"}
	default:
		return []string{}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON encode error: %v", err)
	}
}

func errResp(msg string) map[string]string {
	return map[string]string{"error": msg}
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
