package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"smarthome/db"
	"smarthome/models"
	"smarthome/services"

	"github.com/gin-gonic/gin"
)

// SensorHandler handles sensor-related requests
type SensorHandler struct {
	DB                 *db.DB
	TemperatureService *services.TemperatureService
	DeviceService      *services.DeviceService
	TelemetryService   *services.TelemetryService
}

// NewSensorHandler creates a new SensorHandler
func NewSensorHandler(
	db *db.DB,
	temperatureService *services.TemperatureService,
	DeviceService *services.DeviceService,
	TelemetryService *services.TelemetryService,
) *SensorHandler {
	return &SensorHandler{
		DB:                 db,
		TemperatureService: temperatureService,
		DeviceService:      DeviceService,
		TelemetryService:   TelemetryService,
	}
}

// RegisterRoutes registers the sensor routes
func (h *SensorHandler) RegisterRoutes(router *gin.RouterGroup) {
	sensors := router.Group("/sensors")
	{
		sensors.GET("", h.GetSensors)
		sensors.GET("/:id", h.GetSensorByID)
		sensors.POST("", h.CreateSensor)
		sensors.PUT("/:id", h.UpdateSensor)
		sensors.DELETE("/:id", h.DeleteSensor)
		sensors.PATCH("/:id/value", h.UpdateSensorValue)
		sensors.GET("/:id/device", h.GetSensorDevice)
		sensors.GET("/:id/telemetry", h.GetSensorTelemetry)
		sensors.GET("/:id/telemetry/latest", h.GetSensorLatestTelemetry)
		sensors.GET("/:id/microservices-summary", h.GetSensorMicroservicesSummary)
		sensors.GET("/temperature/:location", h.GetTemperatureByLocation)
	}
}

// GetSensors handles GET /api/v1/sensors
func (h *SensorHandler) GetSensors(c *gin.Context) {
	sensors, err := h.DB.GetSensors(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update temperature sensors with real-time data from the external API
	for i, sensor := range sensors {
		if sensor.Type == models.Temperature {
			tempData, err := h.TemperatureService.GetTemperatureByID(fmt.Sprintf("%d", sensor.ID))
			if err == nil {
				// Update sensor with real-time data
				sensors[i].Value = tempData.Value
				sensors[i].Status = tempData.Status
				sensors[i].LastUpdated = tempData.Timestamp
				log.Printf("Updated temperature data for sensor %d from external API", sensor.ID)

				h.asyncRecordTelemetry(
					fmt.Sprintf("%d", sensor.ID),
					tempData.Value,
					tempData.Unit,
					sensor.Location,
					tempData.Timestamp,
				)
			} else {
				log.Printf("Failed to fetch temperature data for sensor %d: %v", sensor.ID, err)
			}
		}
	}

	c.JSON(http.StatusOK, sensors)
}

// GetSensorByID handles GET /api/v1/sensors/:id
func (h *SensorHandler) GetSensorByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sensor ID"})
		return
	}

	sensor, err := h.DB.GetSensorByID(context.Background(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sensor not found"})
		return
	}

	// If this is a temperature sensor, fetch real-time data from the temperature API
	if sensor.Type == models.Temperature {
		tempData, err := h.TemperatureService.GetTemperatureByID(fmt.Sprintf("%d", sensor.ID))
		if err == nil {
			// Update sensor with real-time data
			sensor.Value = tempData.Value
			sensor.Status = tempData.Status
			sensor.LastUpdated = tempData.Timestamp
			log.Printf("Updated temperature data for sensor %d from external API", sensor.ID)

			h.asyncRecordTelemetry(
				fmt.Sprintf("%d", sensor.ID),
				tempData.Value,
				tempData.Unit,
				sensor.Location,
				tempData.Timestamp,
			)
		} else {
			log.Printf("Failed to fetch temperature data for sensor %d: %v", sensor.ID, err)
		}
	}

	c.JSON(http.StatusOK, sensor)
}

// GetTemperatureByLocation handles GET /api/v1/sensors/temperature/:location
func (h *SensorHandler) GetTemperatureByLocation(c *gin.Context) {
	location := c.Param("location")
	if location == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Location is required"})
		return
	}

	// Fetch temperature data from the external API
	tempData, err := h.TemperatureService.GetTemperature(location)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to fetch temperature data: %v", err),
		})
		return
	}

	// Return the temperature data
	c.JSON(http.StatusOK, gin.H{
		"location":    tempData.Location,
		"value":       tempData.Value,
		"unit":        tempData.Unit,
		"status":      tempData.Status,
		"timestamp":   tempData.Timestamp,
		"description": tempData.Description,
	})
}

// CreateSensor handles POST /api/v1/sensors
func (h *SensorHandler) CreateSensor(c *gin.Context) {
	var sensorCreate models.SensorCreate
	if err := c.ShouldBindJSON(&sensorCreate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sensor, err := h.DB.CreateSensor(context.Background(), sensorCreate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}


	go h.DeviceService.CreateDevice(
		fmt.Sprintf("%d", sensor.ID),
		sensor.Name,
		string(sensor.Type),
		sensor.Location,
		sensor.Unit,
		sensor.Status,
	)

	c.JSON(http.StatusCreated, sensor)
}

// UpdateSensor handles PUT /api/v1/sensors/:id
func (h *SensorHandler) UpdateSensor(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sensor ID"})
		return
	}

	var sensorUpdate models.SensorUpdate
	if err := c.ShouldBindJSON(&sensorUpdate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sensor, err := h.DB.UpdateSensor(context.Background(), id, sensorUpdate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	go h.DeviceService.UpdateDevice(
		fmt.Sprintf("%d", sensor.ID),
		sensor.Name,
		string(sensor.Type),
		sensor.Location,
		sensor.Unit,
		sensor.Status,
	)

	c.JSON(http.StatusOK, sensor)
}

// DeleteSensor handles DELETE /api/v1/sensors/:id
func (h *SensorHandler) DeleteSensor(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sensor ID"})
		return
	}

	err = h.DB.DeleteSensor(context.Background(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	go h.DeviceService.DeleteDevice(fmt.Sprintf("%d", id))

	c.JSON(http.StatusOK, gin.H{"message": "Sensor deleted successfully"})
}

// UpdateSensorValue handles PATCH /api/v1/sensors/:id/value
func (h *SensorHandler) UpdateSensorValue(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sensor ID"})
		return
	}

	var request struct {
		Value  float64 `json:"value" binding:"required"`
		Status string  `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = h.DB.UpdateSensorValue(context.Background(), id, request.Value, request.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	go func() {
		sensor, err := h.DB.GetSensorByID(context.Background(), id)
		if err != nil {
			log.Printf("UpdateSensorValue: failed to fetch sensor %d for telemetry: %v", id, err)
			return
		}
		if sensor.Type == models.Temperature {
			h.TelemetryService.RecordTemperature(
				fmt.Sprintf("%d", id),
				request.Value,
				sensor.Unit,
				sensor.Location,
				time.Now().UTC(),
			)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Sensor value updated successfully"})
}

// GetSensorDevice handles GET /api/v1/sensors/:id/device
func (h *SensorHandler) GetSensorDevice(c *gin.Context) {
	sensorID := c.Param("id")
	if _, err := strconv.Atoi(sensorID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sensor ID"})
		return
	}

	device, err := h.DeviceService.GetDeviceByLegacySensorID(sensorID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to fetch device from device-service: %v", err)})
		return
	}

	c.JSON(http.StatusOK, device)
}

// GetSensorLatestTelemetry handles GET /api/v1/sensors/:id/telemetry/latest
func (h *SensorHandler) GetSensorLatestTelemetry(c *gin.Context) {
	sensorID := c.Param("id")
	if _, err := strconv.Atoi(sensorID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sensor ID"})
		return
	}

	record, err := h.TelemetryService.GetLatestTelemetry(sensorID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to fetch latest telemetry from telemetry-service: %v", err)})
		return
	}

	c.JSON(http.StatusOK, record)
}

// GetSensorTelemetry handles GET /api/v1/sensors/:id/telemetry
func (h *SensorHandler) GetSensorTelemetry(c *gin.Context) {
	sensorID := c.Param("id")
	if _, err := strconv.Atoi(sensorID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sensor ID"})
		return
	}

	metric := c.DefaultQuery("metric", "temperature")
	records, err := h.TelemetryService.ListTelemetry(sensorID, metric)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to fetch telemetry from telemetry-service: %v", err)})
		return
	}

	c.JSON(http.StatusOK, records)
}

// GetSensorMicroservicesSummary handles GET /api/v1/sensors/:id/microservices-summary
func (h *SensorHandler) GetSensorMicroservicesSummary(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sensor ID"})
		return
	}

	sensor, err := h.DB.GetSensorByID(context.Background(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sensor not found"})
		return
	}

	sensorID := fmt.Sprintf("%d", sensor.ID)

	device, deviceErr := h.DeviceService.GetDeviceByLegacySensorID(sensorID)
	latestTelemetry, telemetryErr := h.TelemetryService.GetLatestTelemetry(sensorID)

	response := gin.H{
		"sensor": sensor,
		"device_service": gin.H{
			"device": device,
		},
		"telemetry_service": gin.H{
			"latest": latestTelemetry,
		},
	}

	if deviceErr != nil {
		response["device_service"].(gin.H)["error"] = deviceErr.Error()
	}
	if telemetryErr != nil {
		response["telemetry_service"].(gin.H)["error"] = telemetryErr.Error()
	}

	c.JSON(http.StatusOK, response)
}

// asyncRecordTelemetry fires off a telemetry recording in the background
func (h *SensorHandler) asyncRecordTelemetry(sensorID string, value float64, unit, location string, observedAt time.Time) {
	go h.TelemetryService.RecordTemperature(sensorID, value, unit, location, observedAt)
}
