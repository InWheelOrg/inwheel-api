/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/InWheelOrg/inwheel-server/internal/a11y"
	"github.com/InWheelOrg/inwheel-server/internal/db"
	"github.com/InWheelOrg/inwheel-server/internal/geo"
	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Server handles the HTTP requests for the InWheel API.
type Server struct {
	db     *gorm.DB
	engine *a11y.Engine
}

// main initializes the database connection, runs migrations, and starts the public API server.
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

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
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}

	if err := db.Migrate(gormDB); err != nil {
		slog.Error("Failed to migrate database", "error", err)
		os.Exit(1)
	}

	srv := &Server{db: gormDB, engine: &a11y.Engine{}}

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

	slog.Info("Starting Public API", "addr", srvAddr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("HTTP server failed", "error", err)
		os.Exit(1)
	}
}

// handleGetPlaces handles requests for a list of places, supporting two types of spatial filters.
//
// If 'lng', 'lat', and 'radius' are provided, it performs a circular proximity search.
// If 'min_lng', 'min_lat', 'max_lng', and 'max_lat' are provided, it performs a bounding box search.
//
// If no spatial parameters are present, it defaults to returning the most recently updated 100 places.
// It returns a 400 Bad Request if coordinate parameters are present but malformed.
func (s *Server) handleGetPlaces(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lngStr := q.Get("lng")
	latStr := q.Get("lat")
	radiusStr := q.Get("radius")

	var places []models.Place
	var err error

	if lngStr != "" && latStr != "" && radiusStr != "" {
		lng, ok1 := parseCoord(w, lngStr, "longitude")
		lat, ok2 := parseCoord(w, latStr, "latitude")
		radius, ok3 := parseCoord(w, radiusStr, "radius")

		if !ok1 || !ok2 || !ok3 {
			return
		}
		places, err = geo.FindNearbyPlaces(s.db, lng, lat, radius)
	} else if q.Get("min_lng") != "" {
		minLng, ok1 := parseCoord(w, q.Get("min_lng"), "min_lng")
		minLat, ok2 := parseCoord(w, q.Get("min_lat"), "min_lat")
		maxLng, ok3 := parseCoord(w, q.Get("max_lng"), "max_lng")
		maxLat, ok4 := parseCoord(w, q.Get("max_lat"), "max_lat")

		if !ok1 || !ok2 || !ok3 || !ok4 {
			return
		}
		places, err = geo.FindPlacesInBoundingBox(s.db, minLng, minLat, maxLng, maxLat)
	} else {
		err = s.db.Preload("Accessibility").Order("updated_at DESC").Limit(100).Find(&places).Error
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
// If accessibility data is included, it is validated and audit flags computed before insert.
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
		place.Accessibility.UpdatedAt = time.Now()
		s.engine.WithAuditFlags(place.Accessibility)
		if conflicts := s.engine.DetectConflicts(place.Accessibility); len(conflicts) > 0 {
			jsonResponse(w, conflictError(conflicts), http.StatusUnprocessableEntity)
			return
		}
	}

	if err := s.db.Create(&place).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, place, http.StatusCreated)
}

// handlePatchAccessibility updates or creates the accessibility profile for a specific place.
// Audit flags are computed from submitted component properties and conflicts return 422.
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

	s.engine.WithAuditFlags(&input)
	if conflicts := s.engine.DetectConflicts(&input); len(conflicts) > 0 {
		jsonResponse(w, conflictError(conflicts), http.StatusUnprocessableEntity)
		return
	}

	var result models.AccessibilityProfile
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var profile models.AccessibilityProfile
		err := tx.Where("place_id = ?", id).First(&profile).Error

		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := tx.First(&models.Place{}, "id = ?", id).Error; err != nil {
				return err // ErrRecordNotFound propagates as-is 404, other errors 500
			}
			input.PlaceID = id
			input.UpdatedAt = time.Now()
			if err := tx.Create(&input).Error; err != nil {
				return err
			}
			result = input
			return nil
		}

		updates := map[string]any{
			"overall_status": input.OverallStatus,
			"components":     input.Components,
			"updated_at":     time.Now(),
		}

		if err := tx.Model(&profile).Clauses(clause.Returning{}).Updates(updates).Error; err != nil {
			return err
		}
		result = profile
		return nil
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "Place not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, result, http.StatusOK)
}

// conflictError builds the 422 response body from a slice of detected conflicts.
func conflictError(conflicts []a11y.Conflict) any {
	type conflictItem struct {
		Component string `json:"component"`
		Reason    string `json:"reason"`
	}
	items := make([]conflictItem, len(conflicts))
	for i, c := range conflicts {
		items[i] = conflictItem{Component: string(c.Component), Reason: c.Reason}
	}
	return struct {
		Error     string         `json:"error"`
		Conflicts []conflictItem `json:"conflicts"`
	}{
		Error:     "accessibility data contains conflicts",
		Conflicts: items,
	}
}

// jsonResponse is a helper to write a JSON response to the client.
func jsonResponse(w http.ResponseWriter, data any, code int) {
	w.Header().Set("Content-Type", "application/json")

	payload, err := json.Marshal(data)
	if err != nil {
		slog.Error("Error marshaling JSON response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(code)
	if _, err := w.Write(payload); err != nil {
		slog.Error("Error writing JSON response", "error", err)
	}
}

// parseCoord attempts to parse a string into a float64.
// If parsing fails, it writes an HTTP 400 error to the provided ResponseWriter
// and returns (0, false). The caller must check the boolean return value
// before proceeding with the parsed result.
func parseCoord(w http.ResponseWriter, val string, name string) (float64, bool) {
	f, err := strconv.ParseFloat(val, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		http.Error(w, "Invalid "+name, http.StatusBadRequest)
		return 0, false
	}
	return f, true
}

// getEnv is a helper to read an environment variable or return a fallback value.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
