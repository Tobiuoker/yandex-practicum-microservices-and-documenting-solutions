package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TelemetryRecord is a single stored measurement.
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

// TelemetryCreateRequest is the body for POST /telemetry.
type TelemetryCreateRequest struct {
	LegacySensorID string    `json:"legacy_sensor_id"`
	DeviceID       *string   `json:"device_id,omitempty"`
	Metric         string    `json:"metric"`
	Value          float64   `json:"value"`
	Unit           string    `json:"unit"`
	Location       string    `json:"location"`
	ObservedAt     time.Time `json:"observed_at"`
}

type server struct {
	pool *pgxpool.Pool
}

func main() {
	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/telemetry")
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
	mux.HandleFunc("POST /telemetry", s.handleCreateTelemetry)
	mux.HandleFunc("GET /telemetry/latest", s.handleLatestTelemetry)
	mux.HandleFunc("GET /telemetry", s.handleListTelemetry)

	port := getEnv("PORT", ":8083")
	log.Printf("telemetry-service listening on %s", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleCreateTelemetry stores a new telemetry record
func (s *server) handleCreateTelemetry(w http.ResponseWriter, r *http.Request) {
	var req TelemetryCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid JSON: "+err.Error()))
		return
	}
	if req.LegacySensorID == "" {
		writeJSON(w, http.StatusBadRequest, errResp("legacy_sensor_id is required"))
		return
	}
	if req.Metric == "" {
		req.Metric = "temperature"
	}
	if req.ObservedAt.IsZero() {
		req.ObservedAt = time.Now().UTC()
	}

	record, err := s.insertTelemetry(r.Context(), req)
	if err != nil {
		log.Printf("insertTelemetry error: %v", err)
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *server) handleListTelemetry(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sensorID := q.Get("legacy_sensor_id")
	metric := q.Get("metric")

	var from, to *time.Time
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errResp("invalid 'from' timestamp, use RFC3339"))
			return
		}
		from = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errResp("invalid 'to' timestamp, use RFC3339"))
			return
		}
		to = &t
	}

	records, err := s.queryTelemetry(r.Context(), sensorID, metric, from, to)
	if err != nil {
		log.Printf("queryTelemetry error: %v", err)
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	if records == nil {
		records = []TelemetryRecord{}
	}
	writeJSON(w, http.StatusOK, records)
}

// handleLatestTelemetry returns the most recent record for a sensor
func (s *server) handleLatestTelemetry(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("legacy_sensor_id")
	if sensorID == "" {
		writeJSON(w, http.StatusBadRequest, errResp("legacy_sensor_id query param is required"))
		return
	}

	record, err := s.latestTelemetry(r.Context(), sensorID)
	if err != nil {
		log.Printf("latestTelemetry error: %v", err)
		writeJSON(w, http.StatusNotFound, errResp("no telemetry found for sensor"))
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *server) insertTelemetry(ctx context.Context, req TelemetryCreateRequest) (TelemetryRecord, error) {
	query := `
		INSERT INTO telemetry (legacy_sensor_id, device_id, metric, value, unit, location, observed_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		RETURNING id, legacy_sensor_id, device_id, metric, value, unit, location, observed_at, created_at
	`
	row := s.pool.QueryRow(ctx, query,
		req.LegacySensorID, req.DeviceID, req.Metric, req.Value, req.Unit, req.Location, req.ObservedAt,
	)
	return scanRecord(row)
}

func (s *server) queryTelemetry(ctx context.Context, sensorID, metric string, from, to *time.Time) ([]TelemetryRecord, error) {
	query := `
		SELECT id, legacy_sensor_id, device_id, metric, value, unit, location, observed_at, created_at
		FROM telemetry
		WHERE ($1 = '' OR legacy_sensor_id = $1)
		  AND ($2 = '' OR metric = $2)
		  AND ($3::timestamptz IS NULL OR observed_at >= $3)
		  AND ($4::timestamptz IS NULL OR observed_at <= $4)
		ORDER BY observed_at DESC
		LIMIT 1000
	`
	rows, err := s.pool.Query(ctx, query, sensorID, metric, from, to)
	if err != nil {
		return nil, fmt.Errorf("query telemetry: %w", err)
	}
	defer rows.Close()

	var records []TelemetryRecord
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *server) latestTelemetry(ctx context.Context, sensorID string) (TelemetryRecord, error) {
	query := `
		SELECT id, legacy_sensor_id, device_id, metric, value, unit, location, observed_at, created_at
		FROM telemetry
		WHERE legacy_sensor_id = $1
		ORDER BY observed_at DESC
		LIMIT 1
	`
	row := s.pool.QueryRow(ctx, query, sensorID)
	return scanRecord(row)
}

func scanRecord(row interface {
	Scan(dest ...any) error
}) (TelemetryRecord, error) {
	var rec TelemetryRecord
	err := row.Scan(
		&rec.ID,
		&rec.LegacySensorID,
		&rec.DeviceID,
		&rec.Metric,
		&rec.Value,
		&rec.Unit,
		&rec.Location,
		&rec.ObservedAt,
		&rec.CreatedAt,
	)
	return rec, err
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
