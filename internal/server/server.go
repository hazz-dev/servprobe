package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/hazz-dev/svcmon/internal/config"
	"github.com/hazz-dev/svcmon/internal/storage"
)

// ServerStore defines the storage queries the server needs.
type ServerStore interface {
	AllLatest(ctx context.Context) ([]storage.Check, error)
	LatestCheck(ctx context.Context, service string) (*storage.Check, error)
	ServiceHistory(ctx context.Context, service string, limit, offset int) ([]storage.Check, int, error)
	UptimePercent(ctx context.Context, service string, last int) (float64, error)
}

// Server holds the chi router and its dependencies.
type Server struct {
	store    ServerStore
	services []config.Service
	router   chi.Router
	logger   *slog.Logger
}

// New creates a new Server and registers all routes.
func New(store ServerStore, services []config.Service, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		store:    store,
		services: services,
		router:   chi.NewRouter(),
		logger:   logger,
	}
	s.registerRoutes()
	return s
}

// Router returns the chi router (for mounting or testing).
func (s *Server) Router() chi.Router {
	return s.router
}

func (s *Server) registerRoutes() {
	r := s.router
	r.Use(middleware.Recoverer)
	r.Use(s.requestLogger)

	r.Get("/api/health", s.handleHealth)
	r.Get("/api/services", s.handleListServices)
	r.Get("/api/services/{name}", s.handleGetService)
	r.Get("/api/services/{name}/history", s.handleGetServiceHistory)
}

// --- Response helpers ---

type envelope struct {
	Data  interface{} `json:"data"`
	Error string      `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(envelope{Data: data})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(envelope{Error: msg})
}

// --- Service helpers ---

// serviceIndex returns a map from service name → config.Service.
func (s *Server) serviceIndex() map[string]config.Service {
	idx := make(map[string]config.Service, len(s.services))
	for _, svc := range s.services {
		idx[svc.Name] = svc
	}
	return idx
}

// --- Handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

type serviceDetail struct {
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	Target      string     `json:"target"`
	Interval    string     `json:"interval"`
	Status      string     `json:"status"`
	ResponseMs  int64      `json:"response_ms"`
	UptimePct   float64    `json:"uptime_percent"`
	LastChecked *time.Time `json:"last_checked"`
}

func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	latestChecks, err := s.store.AllLatest(r.Context())
	if err != nil {
		s.logger.Error("AllLatest", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	byService := make(map[string]storage.Check, len(latestChecks))
	for _, c := range latestChecks {
		byService[c.Service] = c
	}

	details := make([]serviceDetail, 0, len(s.services))
	for _, svc := range s.services {
		d := serviceDetail{
			Name:     svc.Name,
			Type:     svc.Type,
			Target:   svc.Target,
			Interval: svc.Interval.Duration.String(),
			Status:   "unknown",
		}
		if c, ok := byService[svc.Name]; ok {
			d.Status = c.Status
			d.ResponseMs = c.ResponseMs
			t := c.CheckedAt
			d.LastChecked = &t
			pct, _ := s.store.UptimePercent(r.Context(), svc.Name, 100)
			d.UptimePct = pct
		}
		details = append(details, d)
	}

	writeJSON(w, http.StatusOK, details)
}

type serviceDetailResponse struct {
	serviceDetail
	RecentChecks []storage.Check `json:"recent_checks"`
}

func (s *Server) handleGetService(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	idx := s.serviceIndex()
	svc, ok := idx[name]
	if !ok {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}

	latest, err := s.store.LatestCheck(r.Context(), name)
	if err != nil {
		s.logger.Error("LatestCheck", "service", name, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	history, _, err := s.store.ServiceHistory(r.Context(), name, 10, 0)
	if err != nil {
		s.logger.Error("ServiceHistory", "service", name, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	pct, _ := s.store.UptimePercent(r.Context(), name, 100)

	d := serviceDetail{
		Name:      svc.Name,
		Type:      svc.Type,
		Target:    svc.Target,
		Interval:  svc.Interval.Duration.String(),
		Status:    "unknown",
		UptimePct: pct,
	}
	if latest != nil {
		d.Status = latest.Status
		d.ResponseMs = latest.ResponseMs
		t := latest.CheckedAt
		d.LastChecked = &t
	}

	writeJSON(w, http.StatusOK, serviceDetailResponse{
		serviceDetail: d,
		RecentChecks:  history,
	})
}

type historyResponse struct {
	Checks []storage.Check `json:"checks"`
	Total  int             `json:"total"`
}

func (s *Server) handleGetServiceHistory(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	idx := s.serviceIndex()
	if _, ok := idx[name]; !ok {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}

	const maxLimit = 1000

	limit := 50
	offset := 0

	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid limit parameter")
			return
		}
		if n > maxLimit {
			n = maxLimit
		}
		limit = n
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid offset parameter")
			return
		}
		offset = n
	}

	checks, total, err := s.store.ServiceHistory(r.Context(), name, limit, offset)
	if err != nil {
		s.logger.Error("ServiceHistory", "service", name, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, historyResponse{
		Checks: checks,
		Total:  total,
	})
}

// --- Middleware ---

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start),
		)
	})
}
