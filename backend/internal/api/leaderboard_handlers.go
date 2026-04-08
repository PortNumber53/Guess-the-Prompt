package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"guessapi/internal/db"
)

type LeaderboardHandler struct {
	DB *db.Database
}

func (h *LeaderboardHandler) RegisterRoutes(r chi.Router) {
	r.Get("/leaderboard", h.GetLeaderboard)
}

type LeaderboardEntry struct {
	Rank           int    `json:"rank"`
	Username       string `json:"name"`
	PromptsGuessed int    `json:"promptsGuessed"`
	CoinsEarned    int    `json:"coinsEarned"`
	WinRate        string `json:"winRate"`
	Avatar         string `json:"avatar"`
}

func (h *LeaderboardHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
    // Top 5 users based on guess_coins
	rows, err := h.DB.Pool.Query(r.Context(), "SELECT id, username, guess_coins FROM users ORDER BY guess_coins DESC LIMIT 5")
	if err != nil {
		http.Error(w, "Failed to fetch leaderboard", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var leaderboard []LeaderboardEntry
	rank := 1
	for rows.Next() {
		var id int
		var username string
		var coins int
		
		if err := rows.Scan(&id, &username, &coins); err != nil {
			http.Error(w, "Error scanning user", http.StatusInternalServerError)
			return
		}

        // Mocking the avatar and winRate dynamically since those tables aren't completely built
		entry := LeaderboardEntry{
			Rank:           rank,
			Username:       username,
			PromptsGuessed: (coins / 500) + 1, // rough mock metric
			CoinsEarned:    coins,
			WinRate:        "50%",
			Avatar:         "https://i.pravatar.cc/150?u=" + username,
		}
		leaderboard = append(leaderboard, entry)
		rank++
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(leaderboard)
}
