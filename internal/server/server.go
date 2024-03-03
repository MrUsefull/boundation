package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/MrUsefull/boundation/internal/config"
	"github.com/MrUsefull/boundation/internal/unbound"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

const (
	RecordsEndpoint string = "/records"
	AdjustEndpoint  string = "/adjustendpoints"

	MediaType string = "application/external.dns.webhook+json;version=1"
)

type Opts func(*Server)

func WithProvider(p provider.Provider) Opts {
	return func(s *Server) {
		s.provider = p
	}
}

type Server struct {
	log      *slog.Logger
	cfg      config.Config
	provider provider.Provider
}

func New(cfg config.Config, log *slog.Logger, opts ...Opts) *Server {
	s := &Server{
		log:      log,
		cfg:      cfg,
		provider: unbound.New(http.DefaultClient, cfg, log),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func Serve(ctx context.Context, cfg config.Config, log *slog.Logger) {
	server := New(cfg, log)
	DoServe(ctx, server)
}

func DoServe(ctx context.Context, server *Server) {
	shutdownFN := server.ListenAndServe(ctx, server.Routes())

	<-ctx.Done()
	shutdownFN()
}

func (s Server) Routes() *chi.Mux {
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.SetHeader("Content-Type", MediaType))

	router.Get("/", filterHandler(s.provider, s.log))

	router.Get("/healthz", healthCheck())

	router.Get(RecordsEndpoint, recordsHandler(s.provider, s.log))
	router.Post(RecordsEndpoint, applyHandler(s.provider, s.log))

	router.Post(AdjustEndpoint, adjustEndpointsHandler(s.provider, s.log))

	return router
}

func (s Server) ListenAndServe(ctx context.Context, router *chi.Mux) func() {
	httpServer := http.Server{
		Addr:              s.cfg.Addr,
		ReadHeaderTimeout: 3 * time.Second,
		Handler:           router,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			s.log.ErrorContext(ctx, "error serving", slog.Any("error", err))
		}
	}()

	return func() {
		if err := httpServer.Shutdown(ctx); err != nil {
			s.log.ErrorContext(ctx, "issue shutting down server", slog.Any("error", err))
		}
	}
}

func filterHandler(provider provider.Provider, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		filter := provider.GetDomainFilter()
		writeJSON(ctx, w, filter, log)
	}
}

func recordsHandler(provider provider.Provider, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		endpoints, err := provider.Records(ctx)
		if err != nil {
			log.ErrorContext(ctx, "error getting records", slog.Any("err", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		writeJSON(ctx, w, endpoints, log)
	}
}

func applyHandler(provider provider.Provider, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()

		ctx := request.Context()

		thePlan := &plan.Changes{}

		err := json.NewDecoder(request.Body).Decode(thePlan)
		if err != nil {
			log.ErrorContext(ctx, "failed to unmarshal the plan", slog.Any("error", err))
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		if err := provider.ApplyChanges(ctx, thePlan); err != nil {
			log.ErrorContext(ctx, "failed to apply the plan", slog.Any("error", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func adjustEndpointsHandler(provider provider.Provider, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		endpoints := []*endpoint.Endpoint{}
		if err := json.NewDecoder(r.Body).Decode(&endpoints); err != nil {
			log.ErrorContext(ctx, "failed to unmarshal endpoints", slog.Any("error", err))
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		results, err := provider.AdjustEndpoints(endpoints)
		if err != nil {
			log.ErrorContext(ctx, "failed to adjust enpoints", slog.Any("error", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		writeJSON(ctx, w, results, log)
	}
}

func healthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

func writeJSON(ctx context.Context, w http.ResponseWriter, output any, log *slog.Logger) {
	jsonBytes, err := json.Marshal(output)
	if err != nil {
		log.ErrorContext(ctx, "error marshalling records", slog.Any("err", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	if _, err := w.Write(jsonBytes); err != nil {
		log.ErrorContext(ctx, "error writing response", slog.Any("err", err))
	}
}
