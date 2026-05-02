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

	apispec "github.com/InWheelOrg/inwheel-server/api"
	"github.com/InWheelOrg/inwheel-server/internal/a11y"
	apiv1 "github.com/InWheelOrg/inwheel-server/internal/api/v1"
	"github.com/InWheelOrg/inwheel-server/internal/db"
	"github.com/InWheelOrg/inwheel-server/internal/geo"
	"github.com/InWheelOrg/inwheel-server/internal/middleware"
	"github.com/InWheelOrg/inwheel-server/internal/pagination"
	"github.com/InWheelOrg/inwheel-server/internal/validation"
	"github.com/InWheelOrg/inwheel-server/pkg/models"
	"github.com/getkin/kin-openapi/openapi3filter"
	nethttp_middleware "github.com/oapi-codegen/nethttp-middleware"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ctxKeyRequest struct{}

func requestFromCtx(ctx context.Context) *http.Request {
	r, _ := ctx.Value(ctxKeyRequest{}).(*http.Request)
	return r
}

func injectRequest() apiv1.StrictMiddlewareFunc {
	return func(f apiv1.StrictHandlerFunc, operationID string) apiv1.StrictHandlerFunc {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req interface{}) (interface{}, error) {
			return f(context.WithValue(ctx, ctxKeyRequest{}, r), w, r, req)
		}
	}
}

// Server handles HTTP requests for the InWheel API and implements StrictServerInterface.
type Server struct {
	db         *gorm.DB
	engine     *a11y.Engine
	regLimiter *middleware.RateLimiter
	keyLimiter *middleware.RateLimiter
}

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

	srv := &Server{
		db:         gormDB,
		engine:     &a11y.Engine{},
		regLimiter: middleware.NewRateLimiter(ctx, rate.Every(20*time.Minute), 3),
		keyLimiter: middleware.NewRateLimiter(ctx, rate.Every(time.Second), 60),
	}

	// v1Mux holds only /v1/* routes so the spec validator only wraps those.
	// Unversioned routes (/healthz, /readyz, /openapi.yaml) are registered on
	// the outer mux and never go through the validator.
	v1Mux := http.NewServeMux()

	swagger, err := apiv1.GetSpec()
	if err != nil {
		slog.Error("Failed to load OpenAPI spec", "error", err)
		os.Exit(1)
	}

	strictHandler := apiv1.NewStrictHandlerWithOptions(srv, []apiv1.StrictMiddlewareFunc{injectRequest()}, apiv1.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  srv.validationErrorHandler,
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			slog.Error("handler error", "error", err)
			writeJSON(w, map[string]string{"error": "internal server error"}, http.StatusInternalServerError)
		},
	})
	apiv1.HandlerWithOptions(strictHandler, apiv1.StdHTTPServerOptions{
		BaseURL:          "/v1",
		BaseRouter:       v1Mux,
		ErrorHandlerFunc: srv.validationErrorHandler,
		Middlewares: []apiv1.MiddlewareFunc{
			bodySizeLimiter(1 << 20),
		},
	})

	v1Handler := nethttp_middleware.OapiRequestValidatorWithOptions(swagger, &nethttp_middleware.Options{
		SilenceServersWarning: true,
		Options: openapi3filter.Options{
			AuthenticationFunc: srv.authenticate,
		},
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request, _ nethttp_middleware.ErrorHandlerOpts) {
			srv.validationErrorHandler(w, r, err)
		},
	})(v1Mux)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", srv.handleHealthz)
	mux.HandleFunc("GET /readyz", srv.handleReadyz)
	mux.HandleFunc("GET /openapi.yaml", srv.handleOpenAPISpec)
	mux.Handle("/v1/", v1Handler)

	finalHandler := http.Handler(mux)

	port := getEnv("PORT", "8080")
	httpServer := &http.Server{
		Addr:         ":" + port,
		Handler:      finalHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("Starting Public API", "addr", ":"+port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop()
	slog.Info("Shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		cancel()
		slog.Error("Server shutdown failed", "error", err)
		os.Exit(1)
	}
	cancel()
}

func bodySizeLimiter(maxBytes int64) apiv1.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// ── StrictServerInterface ─────────────────────────────────────────────────────

func (s *Server) ListPlaces(ctx context.Context, request apiv1.ListPlacesRequestObject) (apiv1.ListPlacesResponseObject, error) {
	q := request.Params

	if errs := validation.PlacesQuery(buildRawQuery(q)); len(errs) > 0 {
		return apiv1.ListPlaces400JSONResponse(validationError(errs)), nil
	}

	limit := 20
	if q.Limit != nil {
		limit = *q.Limit
	}

	scope := s.db.Preload("Accessibility").Order("updated_at DESC, id ASC").Limit(limit + 1)

	if q.Cursor != nil {
		if cursorTS, cursorID, err := pagination.Decode(*q.Cursor); err == nil {
			scope = scope.Where("places.updated_at < ? OR (places.updated_at = ? AND places.id > ?)", cursorTS, cursorTS, cursorID)
		}
	}

	var places []models.Place
	var dbErr error

	switch {
	case q.Lng != nil:
		places, dbErr = geo.FindNearbyPlaces(scope, *q.Lng, *q.Lat, *q.Radius)
	case q.MinLng != nil:
		places, dbErr = geo.FindPlacesInBoundingBox(scope, *q.MinLng, *q.MinLat, *q.MaxLng, *q.MaxLat)
	default:
		dbErr = scope.Find(&places).Error
	}

	if dbErr != nil {
		return nil, dbErr
	}

	var nextCursor *string
	if len(places) > limit {
		places = places[:limit]
		last := places[len(places)-1]
		nc := pagination.Encode(last.UpdatedAt, last.ID)
		nextCursor = &nc
	}

	return apiv1.ListPlaces200JSONResponse{Data: places, NextCursor: nextCursor}, nil
}

func (s *Server) CreatePlace(ctx context.Context, request apiv1.CreatePlaceRequestObject) (apiv1.CreatePlaceResponseObject, error) {
	place := *request.Body

	if errs := validation.Place(&place); len(errs) > 0 {
		return apiv1.CreatePlace400JSONResponse(validationError(errs)), nil
	}

	if place.Accessibility != nil {
		place.Accessibility.UpdatedAt = time.Now()
		s.engine.WithAuditFlags(place.Accessibility)
		if conflicts := s.engine.DetectConflicts(place.Accessibility); len(conflicts) > 0 {
			return apiv1.CreatePlace422JSONResponse(conflictError(conflicts)), nil
		}
	}

	if err := s.db.Create(&place).Error; err != nil {
		return nil, err
	}

	return apiv1.CreatePlace201JSONResponse(place), nil
}

func (s *Server) GetPlace(ctx context.Context, request apiv1.GetPlaceRequestObject) (apiv1.GetPlaceResponseObject, error) {
	id := request.Id.String()

	var place models.Place
	if err := s.db.Preload("Accessibility").First(&place, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apiv1.GetPlace404JSONResponse{Error: "place not found"}, nil
		}
		return nil, err
	}

	if place.ParentID != nil {
		var parent models.Place
		if err := s.db.Preload("Accessibility").First(&parent, "id = ?", *place.ParentID).Error; err == nil {
			effective := s.engine.ComputeEffectiveProfile(&place, &parent)
			effective.PlaceID = place.ID
			place.Accessibility = effective
		}
	}

	return apiv1.GetPlace200JSONResponse(place), nil
}

func (s *Server) PatchPlaceAccessibility(ctx context.Context, request apiv1.PatchPlaceAccessibilityRequestObject) (apiv1.PatchPlaceAccessibilityResponseObject, error) {
	id := request.Id.String()
	input := *request.Body

	s.engine.WithAuditFlags(&input)
	if conflicts := s.engine.DetectConflicts(&input); len(conflicts) > 0 {
		return apiv1.PatchPlaceAccessibility422JSONResponse(conflictError(conflicts)), nil
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
				return err
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
			return apiv1.PatchPlaceAccessibility404JSONResponse{Error: "place not found"}, nil
		}
		return nil, err
	}

	return apiv1.PatchPlaceAccessibility200JSONResponse(result), nil
}

// ── Infrastructure handlers ───────────────────────────────────────────────────

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	sqlDB, err := s.db.DB()
	if err != nil || sqlDB.PingContext(ctx) != nil {
		writeJSON(w, map[string]string{"status": "degraded"}, http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	if _, err := w.Write(apispec.SpecYAML); err != nil {
		slog.Error("handleOpenAPISpec: write failed", "error", err)
	}
}

// ── Response type converters ──────────────────────────────────────────────────

func validationError(errs []validation.FieldError) apiv1.ValidationError {
	fields := make([]apiv1.FieldError, len(errs))
	for i, e := range errs {
		fields[i] = apiv1.FieldError{Field: e.Field, Reason: e.Reason}
	}
	return apiv1.ValidationError{Error: "validation failed", Fields: fields}
}

func conflictError(conflicts []a11y.Conflict) apiv1.ConflictError {
	items := make([]apiv1.ConflictItem, len(conflicts))
	for i, c := range conflicts {
		items[i] = apiv1.ConflictItem{Component: string(c.Component), Reason: c.Reason}
	}
	return apiv1.ConflictError{Error: "accessibility data contains conflicts", Conflicts: items}
}

func writeJSON(w http.ResponseWriter, data any, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(data); err != nil {
		slog.Error("writeJSON: encode failed", "error", err)
	}
}

func buildRawQuery(p apiv1.ListPlacesParams) map[string][]string {
	q := make(map[string][]string)
	setFloat := func(key string, v *float64) {
		if v != nil {
			q[key] = []string{strconv.FormatFloat(*v, 'f', -1, 64)}
		}
	}
	setFloat("lng", p.Lng)
	setFloat("lat", p.Lat)
	setFloat("radius", p.Radius)
	setFloat("min_lng", p.MinLng)
	setFloat("min_lat", p.MinLat)
	setFloat("max_lng", p.MaxLng)
	setFloat("max_lat", p.MaxLat)
	if p.Cursor != nil {
		q["cursor"] = []string{*p.Cursor}
	}
	return q
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
