package api

import (
	"net/http"
	"os"

	"guessapi/internal/db"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func NewRouter(database *db.Database) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	fundHandler := &FundHandler{DB: database}

	r.Route("/v1", func(r chi.Router) {
		r.Use(AuthMiddleware)
		r.Use(RateLimitMiddleware)

		// Auth
		authHandler := &AuthHandler{DB: database}
		authHandler.RegisterRoutes(r)

		// Puzzles
		puzzleHandler := &PuzzleHandler{DB: database}
		puzzleHandler.RegisterRoutes(r)

		// Leaderboard
		leaderboardHandler := &LeaderboardHandler{DB: database}
		leaderboardHandler.RegisterRoutes(r)

		// Fund / Purchase (auth-protected endpoints)
		fundHandler.RegisterRoutes(r)
	})

	// Stripe webhook — outside auth middleware (Stripe sends its own signature)
	r.Post("/v1/fund/stripe/webhook", fundHandler.StripeWebhook)

	// WebSocket endpoint for real-time updates
	r.Get("/ws", HandleWebSocket)

	// Serve static files from objects directory
	objectsDir := "./objects"
	if dir := os.Getenv("OBJECTS_DIR"); dir != "" {
		objectsDir = dir
	}
	fileServer := http.FileServer(http.Dir(objectsDir))
	r.Handle("/objects/*", http.StripPrefix("/objects/", fileServer))

	return r
}
