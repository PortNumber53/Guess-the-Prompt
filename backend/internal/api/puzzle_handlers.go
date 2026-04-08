package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"guessapi/internal/db"

	"github.com/go-chi/chi/v5"
)

type PuzzleHandler struct {
	DB *db.Database
}

func (h *PuzzleHandler) RegisterRoutes(r chi.Router) {
	r.Get("/puzzles", h.GetPuzzles)
	r.Get("/puzzles/{id}", h.GetPuzzle)
	r.Post("/puzzles/{id}/guess", h.GuessPuzzle)
	r.Get("/puzzles/{id}/guesses", h.GetGuessHistory)
	// For local dev CORS during fetch
	r.Options("/puzzles/{id}/guess", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

type Puzzle struct {
	ID        int      `json:"id"`
	Prompt    *string  `json:"prompt"`
	PrizePool int      `json:"prize"`
	Winner    *string  `json:"winner"`
	ImageURL  string   `json:"image"`
	Status    string   `json:"status"`
	Tags      []string `json:"tags"`
}

type PuzzlesResponse struct {
	Puzzles []Puzzle `json:"puzzles"`
	Total   int      `json:"total"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
}

func (h *PuzzleHandler) GetPuzzles(w http.ResponseWriter, r *http.Request) {
	limit := 20
	offset := 0
	status := ""

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	if v := r.URL.Query().Get("status"); v == "active" || v == "solved" {
		status = v
	}

	// Count total
	var total int
	countQuery := "SELECT COUNT(*) FROM puzzles"
	if status != "" {
		countQuery += " WHERE status = '" + status + "'"
	}
	h.DB.Pool.QueryRow(r.Context(), countQuery).Scan(&total)

	// Fetch page
	query := "SELECT id, prompt, prize_pool, winner_id, image_url, status, tags FROM puzzles"
	if status != "" {
		query += " WHERE status = '" + status + "'"
	}
	query += " ORDER BY id DESC LIMIT $1 OFFSET $2"

	rows, err := h.DB.Pool.Query(r.Context(), query, limit, offset)
	if err != nil {
		http.Error(w, "Failed to fetch puzzles", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	puzzles := []Puzzle{}
	for rows.Next() {
		var p Puzzle
		var winnerID *int
		var prompt string
		if err := rows.Scan(&p.ID, &prompt, &p.PrizePool, &winnerID, &p.ImageURL, &p.Status, &p.Tags); err != nil {
			http.Error(w, "Error scanning puzzle", http.StatusInternalServerError)
			return
		}

		if p.Status == "active" {
			p.Prompt = nil
		} else {
			p.Prompt = &prompt
		}

		// Mocking winner name based on existence of winnerID
		if winnerID != nil {
			name := "PromptBreaker"
			p.Winner = &name
		}

		puzzles = append(puzzles, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(PuzzlesResponse{
		Puzzles: puzzles,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	})
}

func (h *PuzzleHandler) GetPuzzle(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	puzzleID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid puzzle ID", http.StatusBadRequest)
		return
	}

	var p Puzzle
	var winnerID *int
	var prompt string
	err = h.DB.Pool.QueryRow(r.Context(), "SELECT id, prompt, prize_pool, winner_id, image_url, status, tags FROM puzzles WHERE id = $1", puzzleID).Scan(&p.ID, &prompt, &p.PrizePool, &winnerID, &p.ImageURL, &p.Status, &p.Tags)
	if err != nil {
		http.Error(w, "Puzzle not found", http.StatusNotFound)
		return
	}

	if p.Status == "active" {
		p.Prompt = nil
	} else {
		p.Prompt = &prompt
	}
	if winnerID != nil {
		name := "PromptBreaker"
		p.Winner = &name
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

type GuessRequest struct {
	Words string `json:"words"`
}

type GuessResponse struct {
	Matches      int    `json:"matches"`
	Cost         int    `json:"cost"`
	Type         string `json:"type"`
	Success      bool   `json:"success"`
	NewPrizePool int    `json:"newPrizePool"`
}

func (h *PuzzleHandler) GuessPuzzle(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	puzzleID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid puzzle ID", http.StatusBadRequest)
		return
	}

	var req GuessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	var secretPrompt string
	var status string
	err = h.DB.Pool.QueryRow(r.Context(), "SELECT prompt, status FROM puzzles WHERE id = $1", puzzleID).Scan(&secretPrompt, &status)
	if err != nil {
		http.Error(w, "Puzzle not found", http.StatusNotFound)
		return
	}

	if status != "active" {
		http.Error(w, "Puzzle is already solved", http.StatusBadRequest)
		return
	}

	userID := r.Context().Value(UserIDKey)
	if userID == nil {
		http.Error(w, "Unauthorized. Must be logged in to guess.", http.StatusUnauthorized)
		return
	}

	var username string
	err = h.DB.Pool.QueryRow(r.Context(), "SELECT username FROM users WHERE id = $1", userID).Scan(&username)
	if err != nil {
		http.Error(w, "User identity corrupted", http.StatusUnauthorized)
		return
	}

	reqWords := strings.Fields(req.Words)
	cost := len(reqWords)

	// Deduct cost from user
	_, err = h.DB.Pool.Exec(r.Context(), "UPDATE users SET guess_coins = guess_coins - $1 WHERE id = $2", cost, userID)
	if err != nil {
		http.Error(w, "Failed to deduct coins", http.StatusInternalServerError)
		return
	}

	// Add guess cost to the puzzle's prize pool
	_, err = h.DB.Pool.Exec(r.Context(), "UPDATE puzzles SET prize_pool = prize_pool + $1 WHERE id = $2", cost, puzzleID)
	if err != nil {
		http.Error(w, "Failed to update prize pool", http.StatusInternalServerError)
		return
	}

	cleanSecret := cleanString(secretPrompt)
	secretWords := strings.Fields(cleanSecret)
	secretMap := make(map[string]bool)
	for _, w := range secretWords {
		secretMap[w] = true
	}

	matches := 0
	for _, w := range reqWords {
		cleanW := cleanString(w)
		if secretMap[cleanW] {
			matches++
			delete(secretMap, cleanW)
		}
	}

	guessType := "wrong"
	if matches > 0 {
		guessType = "partial"
	}

	success := len(secretMap) == 0 && matches == len(secretWords)

	_, _ = h.DB.Pool.Exec(r.Context(), "INSERT INTO puzzle_guesses (puzzle_id, user_string, guessed_words, matches, cost) VALUES ($1, $2, $3, $4, $5)", puzzleID, username, req.Words, matches, cost)

	var newPrizePool int
	h.DB.Pool.QueryRow(r.Context(), "SELECT prize_pool FROM puzzles WHERE id = $1", puzzleID).Scan(&newPrizePool)

	GlobalHub.Broadcast(WSMessage{
		Type:     "prize_update",
		PuzzleID: puzzleID,
		Prize:    newPrizePool,
	})

	GlobalHub.Broadcast(WSMessage{
		Type:     "new_guess",
		PuzzleID: puzzleID,
		Username: username,
		Words:    req.Words,
		Matches:  matches,
	})

	if success {
		// Payout calculation (mock 70% to player)
		winnings := int(float64(newPrizePool) * 0.70)
		h.DB.Pool.Exec(r.Context(), "UPDATE users SET guess_coins = guess_coins + $1 WHERE id = $2", winnings, userID)
		h.DB.Pool.Exec(r.Context(), "UPDATE puzzles SET status = 'solved', winner_id = $1 WHERE id = $2", userID, puzzleID)
	}

	resp := GuessResponse{
		Matches:      matches,
		Cost:         cost,
		Type:         guessType,
		Success:      success,
		NewPrizePool: newPrizePool,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *PuzzleHandler) GetGuessHistory(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	puzzleID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid puzzle ID", http.StatusBadRequest)
		return
	}

	userID := r.Context().Value(UserIDKey)
	if userID == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var username string
	err = h.DB.Pool.QueryRow(r.Context(), "SELECT username FROM users WHERE id = $1", userID).Scan(&username)
	if err != nil {
		http.Error(w, "User not found", http.StatusUnauthorized)
		return
	}

	rows, err := h.DB.Pool.Query(r.Context(), "SELECT guessed_words, matches, cost, created_at FROM puzzle_guesses WHERE puzzle_id = $1 AND user_string = $2 ORDER BY created_at DESC", puzzleID, username)
	if err != nil {
		http.Error(w, "Failed to fetch guess history", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type GuessHistoryItem struct {
		Words   string `json:"words"`
		Matches int    `json:"matches"`
		Cost    int    `json:"cost"`
		Type    string `json:"type"`
	}

	var history []GuessHistoryItem
	for rows.Next() {
		var item GuessHistoryItem
		var createdAt interface{}
		if err := rows.Scan(&item.Words, &item.Matches, &item.Cost, &createdAt); err != nil {
			continue
		}
		if item.Matches > 0 {
			item.Type = "partial"
		} else {
			item.Type = "wrong"
		}
		history = append(history, item)
	}

	if history == nil {
		history = []GuessHistoryItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

func cleanString(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(".,!?\"'-:;()[]{}", r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}
