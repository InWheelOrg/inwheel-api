/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/InWheelOrg/inwheel-server/internal/db"
	"github.com/InWheelOrg/inwheel-server/internal/geo"
	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"gorm.io/gorm"
)

// Server handles the HTTP requests for the InWheel API.
type Server struct {
	db *gorm.DB
}

// main initializes the database connection, runs migrations, and starts the public API server.
func main() {
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort, _ := strconv.Atoi(getEnv("DB_PORT", "5432"))
	dbUser := getEnv("DB_USER", "postgres")
	dbPass := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "inwheel")
	dbSSL := getEnv("DB_SSLMODE", "disable")
	dbMaxOpen, _ := strconv.Atoi(getEnv("DB_MAX_OPEN_CONNS", "25"))
	dbMaxIdle, _ := strconv.Atoi(getEnv("DB_MAX_IDLE_CONNS", "5"))

	gormDB, err := db.Connect(db.Config{
		Host:         dbHost,
		Port:         dbPort,
		User:         dbUser,
		Password:     dbPass,
		Name:         dbName,
		SSLMode:      dbSSL,
		MaxOpenConns: dbMaxOpen,
		MaxIdleConns: dbMaxIdle,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := db.Migrate(gormDB); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	srv := &Server{db: gormDB}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /places", srv.handleGetPlaces)
	mux.HandleFunc("GET /places/{id}", srv.handleGetPlace)
	mux.HandleFunc("POST /places", srv.handlePostPlace)
	mux.HandleFunc("PATCH /places/{id}/accessibility", srv.handlePatchAccessibility)

	port := getEnv("PORT", "8080")
	srvAddr := ":" + port

	httpServer := &http.Server{
		Addr:         srvAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("Starting Public API on :%s...", srvAddr)
	log.Fatal(httpServer.ListenAndServe())
}

// handleGetPlaces returns a list of places based on spatial filters.
// It supports two types of queries:
// - /places?lng={lng}&lat={lat}&radius={radius} (radius in meters)
// - /places?min_lng={min_lng}&min_lat={min_lat}&max_lng={max_lng}&max_lat={max_lat}
// If no spatial filters are provided, it returns the first 100 places from the database.
func (s *Server) handleGetPlaces(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lngStr := q.Get("lng")
	latStr := q.Get("lat")
	radiusStr := q.Get("radius")

	var places []models.Place
	var err error

	if lngStr != "" && latStr != "" && radiusStr != "" {
		lng, _ := strconv.ParseFloat(lngStr, 64)
		lat, _ := strconv.ParseFloat(latStr, 64)
		radius, _ := strconv.ParseFloat(radiusStr, 64)
		places, err = geo.FindNearbyPlaces(s.db, lng, lat, radius)
	} else if q.Get("min_lng") != "" {
		minLng, _ := strconv.ParseFloat(q.Get("min_lng"), 64)
		minLat, _ := strconv.ParseFloat(q.Get("min_lat"), 64)
		maxLng, _ := strconv.ParseFloat(q.Get("max_lng"), 64)
		maxLat, _ := strconv.ParseFloat(q.Get("max_lat"), 64)
		places, err = geo.FindPlacesInBoundingBox(s.db, minLng, minLat, maxLng, maxLat)
	} else {
		err = s.db.Preload("Accessibility").Limit(100).Find(&places).Error
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, places, http.StatusOK)
}

// handleGetPlace returns the full details of a single place, including its accessibility profile.
// Endpoint: GET /places/{id}
func (s *Server) handleGetPlace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var place models.Place
	if err := s.db.Preload("Accessibility").First(&place, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "Place not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, place, http.StatusOK)
}

// handlePostPlace creates a new place in the database.
// If accessibility data is included, it automatically flags the profile for audit.
// Endpoint: POST /places
func (s *Server) handlePostPlace(w http.ResponseWriter, r *http.Request) {
	// Limit the request body to 1 MB (1 << 20 bytes)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var place models.Place
	if err := json.NewDecoder(r.Body).Decode(&place); err != nil {
		http.Error(w, "Payload too large or invalid JSON", http.StatusBadRequest)
		return
	}

	if place.Accessibility != nil {
		place.Accessibility.NeedsAudit = true
		place.Accessibility.UpdatedAt = time.Now()
	}

	if err := s.db.Create(&place).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, place, http.StatusCreated)
}

// handlePatchAccessibility updates or creates the accessibility profile for a specific place.
// It increments the data version and sets needs_audit to true.
// Endpoint: PATCH /places/{id}/accessibility
func (s *Server) handlePatchAccessibility(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Limit the request body to 1 MB (1 << 20 bytes)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var input models.AccessibilityProfile
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Payload too large or invalid JSON", http.StatusBadRequest)
		return
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var profile models.AccessibilityProfile
		err := tx.Where("place_id = ?", id).First(&profile).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				input.PlaceID = id
				input.NeedsAudit = true
				input.DataVersion = 1
				input.UpdatedAt = time.Now()
				return tx.Create(&input).Error
			}
			return err
		}

		updates := map[string]any{
			"overall_status": input.OverallStatus,
			"components":     input.Components,
			"needs_audit":    true,
			"data_version":   profile.DataVersion + 1,
			"updated_at":     time.Now(),
		}

		return tx.Model(&profile).Updates(updates).Error
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var updated models.AccessibilityProfile
	s.db.Where("place_id = ?", id).First(&updated)
	jsonResponse(w, updated, http.StatusOK)
}

// jsonResponse is a helper to write a JSON response to the client.
func jsonResponse(w http.ResponseWriter, data any, code int) {
	w.Header().Set("Content-Type", "application/json")

	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshaling JSON response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(code)
	if _, err := w.Write(payload); err != nil {
		log.Printf("Error writing JSON response: %v", err)
	}
}

// getEnv is a helper to read an environment variable or return a fallback value.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
