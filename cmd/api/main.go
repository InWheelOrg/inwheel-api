/*
 * Copyright (C) 2026 InWheel Contributors
 * SPDX-License-Identifier: AGPL-3.0-only
 */

package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/InWheelOrg/inwheel-server/internal/a11y"
	"github.com/InWheelOrg/inwheel-server/internal/db"
	"github.com/InWheelOrg/inwheel-server/internal/geo"
	"github.com/InWheelOrg/inwheel-server/internal/middleware"
	"github.com/InWheelOrg/inwheel-server/internal/validation"
	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Server handles the HTTP requests for the InWheel API.
type Server struct {
	db         *gorm.DB
	engine     *a11y.Engine
	regLimiter *middleware.RateLimiter // per-IP limit for POST /auth/register
	keyLimiter *middleware.RateLimiter // per-key limit for write endpoints
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &Server{
		db:         gormDB,
		engine:     &a11y.Engine{},
		regLimiter: middleware.NewRateLimiter(ctx, rate.Every(20*time.Minute), 3), // 3 registrations/hour/IP
		keyLimiter: middleware.NewRateLimiter(ctx, rate.Every(time.Second), 60),   // 60 writes/min/key
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", srv.handleHealthz)
	mux.HandleFunc("GET /readyz", srv.handleReadyz)
	mux.HandleFunc("GET /places", srv.handleGetPlaces)
	mux.HandleFunc("GET /places/{id}", srv.handleGetPlace)
	mux.HandleFunc("POST /auth/register", srv.handleRegister)
	mux.HandleFunc("DELETE /auth/keys", srv.handleRevokeKey)
	mux.HandleFunc("POST /places", middleware.RequireAPIKey(gormDB, srv.keyLimiter, srv.handlePostPlace))
	mux.HandleFunc("PATCH /places/{id}/accessibility", middleware.RequireAPIKey(gormDB, srv.keyLimiter, srv.handlePatchAccessibility))

	port := getEnv("PORT", "8080")
	srvAddr := ":" + port

	httpServer := &http.Server{
		Addr:         srvAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("Starting Public API", "addr", srvAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop()
	slog.Info("Shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server shutdown failed", "error", err)
		os.Exit(1)
	}
}

// handleGetPlaces handles requests for a list of places, supporting two types of spatial filters.
//
// If 'lng', 'lat', and 'radius' are provided, it performs a circular proximity search.
// If 'min_lng', 'min_lat', 'max_lng', and 'max_lat' are provided, it performs a bounding box search.
//
// If no spatial parameters are present, it defaults to returning the most recently updated 100 places.
// Returns 400 with a structured field-error list if any query param is malformed or out of bounds.
func (s *Server) handleGetPlaces(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if errs := validation.PlacesQuery(q); len(errs) > 0 {
		jsonResponse(w, validationError(errs), http.StatusBadRequest)
		return
	}

	var places []models.Place
	var err error

	switch {
	case q.Get("lng") != "":
		lng, _ := strconv.ParseFloat(q.Get("lng"), 64)
		lat, _ := strconv.ParseFloat(q.Get("lat"), 64)
		radius, _ := strconv.ParseFloat(q.Get("radius"), 64)
		places, err = geo.FindNearbyPlaces(s.db, lng, lat, radius)
	case q.Get("min_lng") != "":
		minLng, _ := strconv.ParseFloat(q.Get("min_lng"), 64)
		minLat, _ := strconv.ParseFloat(q.Get("min_lat"), 64)
		maxLng, _ := strconv.ParseFloat(q.Get("max_lng"), 64)
		maxLat, _ := strconv.ParseFloat(q.Get("max_lat"), 64)
		places, err = geo.FindPlacesInBoundingBox(s.db, minLng, minLat, maxLng, maxLat)
	default:
		err = s.db.Preload("Accessibility").Order("updated_at DESC").Limit(100).Find(&places).Error
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, places, http.StatusOK)
}

// handleGetPlace returns the full details of a single place, including its accessibility profile.
// When the place has a parent, the returned accessibility is the effective profile: the child's own
// components plus any component types inherited from the parent that the child does not own itself.
// Inherited components carry is_inherited=true and source_id=parent.ID.
// Endpoint: GET /places/{id}
func (s *Server) handleGetPlace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if errs := validation.PlaceID(id); len(errs) > 0 {
		jsonResponse(w, validationError(errs), http.StatusBadRequest)
		return
	}

	var place models.Place
	if err := s.db.Preload("Accessibility").First(&place, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "Place not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if place.ParentID != nil {
		var parent models.Place
		if err := s.db.Preload("Accessibility").First(&parent, "id = ?", *place.ParentID).Error; err == nil {
			effective := s.engine.ComputeEffectiveProfile(&place, &parent)
			effective.PlaceID = place.ID
			place.Accessibility = effective
		}
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

	if errs := validation.Place(&place); len(errs) > 0 {
		jsonResponse(w, validationError(errs), http.StatusBadRequest)
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
	if errs := validation.PlaceID(id); len(errs) > 0 {
		jsonResponse(w, validationError(errs), http.StatusBadRequest)
		return
	}

	// Limit the request body to 1 MB (1 << 20 bytes)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var input models.AccessibilityProfile
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Payload too large or invalid JSON", http.StatusBadRequest)
		return
	}

	if errs := validation.AccessibilityProfile(&input); len(errs) > 0 {
		jsonResponse(w, validationError(errs), http.StatusBadRequest)
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

// handleHealthz is a liveness probe: returns 200 as long as the process is running.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}

// handleReadyz is a readiness probe: returns 200 when the DB is reachable, 503 otherwise.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	sqlDB, err := s.db.DB()
	if err != nil || sqlDB.PingContext(ctx) != nil {
		jsonResponse(w, map[string]string{"status": "degraded"}, http.StatusServiceUnavailable)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}

// validationError builds the 400 response body from a slice of structural-validation errors.
func validationError(errs []validation.FieldError) any {
	return struct {
		Error  string                   `json:"error"`
		Fields []validation.FieldError  `json:"fields"`
	}{
		Error:  "validation failed",
		Fields: errs,
	}
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

// getEnv is a helper to read an environment variable or return a fallback value.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
