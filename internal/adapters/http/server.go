package httpadapter

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"backend-challenge/internal/adapters/oidc"
	"backend-challenge/internal/adapters/sqs"
	"backend-challenge/internal/observability"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"
	"net/http"
)

type Server struct{ httpServer *http.Server }
type postgresReadiness interface{ Ping(context.Context) error }
type sqsReadiness interface{ Ready(context.Context) error }

func NewServer(lc fx.Lifecycle, addr string, pool *pgxpool.Pool, queue *sqs.Client, verifier *oidc.Verifier, business *BusinessHandler, log *slog.Logger, metrics *observability.Metrics) *Server {
	public := http.NewServeMux()
	public.HandleFunc("GET /health/live", live)
	public.HandleFunc("GET /health/ready", readiness(pool, queue))
	private := http.NewServeMux()
	private.HandleFunc("POST /wallets", business.CreateWallet)
	private.HandleFunc("GET /wallets/{walletId}", business.GetWallet)
	private.HandleFunc("GET /wallets/{walletId}/ledger", business.ListLedger)
	private.HandleFunc("POST /wallets/{walletId}/reconciliation", business.Reconcile)
	private.HandleFunc("POST /wagering/transactions", business.ProcessTransaction)
	private.HandleFunc("GET /wagering/transactions/{transactionId}", business.GetTransaction)
	private.HandleFunc("GET /providers/{providerId}/wagering/transactions/{externalTransactionId}", business.GetExternalTransaction)
	privateWithMetrics := withRequestContext(private, log)
	secured := verifier.Middleware(privateWithMetrics)
	root := http.NewServeMux()
	root.Handle("/health/live", public)
	root.Handle("/health/ready", public)
	root.Handle("/metrics", http.HandlerFunc(metrics.Handler))
	root.Handle("/", secured)
	server := &Server{httpServer: &http.Server{Addr: addr, Handler: root, ReadHeaderTimeout: 5 * time.Second}}
	lc.Append(fx.Hook{OnStart: func(context.Context) error { go func() { _ = server.httpServer.ListenAndServe() }(); return nil }, OnStop: func(ctx context.Context) error { return server.httpServer.Shutdown(ctx) }})
	return server
}

func live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "live"})
}
func readiness(pool postgresReadiness, queue sqsReadiness) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "dependency": "postgres"})
			return
		}
		if err := queue.Ready(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "dependency": "sqs"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func withRequestContext(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, correlationID := observability.EnsureCorrelationID(r.Context())
		r = r.WithContext(ctx)
		start := time.Now()
		next.ServeHTTP(w, r)
		log.With("correlationId", correlationID, "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds()).Info("http request completed")
	})
}
