package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"guessapi/internal/config"
	"guessapi/internal/db"

	"github.com/go-chi/chi/v5"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/checkout/session"
	"github.com/stripe/stripe-go/v76/webhook"
)

type Bundle struct {
	ID        int    `json:"id"`
	Coins     int    `json:"coins"`
	BonusPct  int    `json:"bonusPct"`
	PriceCents int   `json:"priceCents"`
	Label     string `json:"label"`
}

// Bundles defines the purchasable coin packages.
// BonusPct is kept for future promotions but set to 0 for launch.
var Bundles = []Bundle{
	{ID: 1, Coins: 100, BonusPct: 0, PriceCents: 100, Label: "100 Coins"},
	{ID: 2, Coins: 500, BonusPct: 0, PriceCents: 400, Label: "500 Coins"},
	{ID: 3, Coins: 1250, BonusPct: 0, PriceCents: 900, Label: "1,250 Coins"},
	{ID: 4, Coins: 3000, BonusPct: 0, PriceCents: 2000, Label: "3,000 Coins"},
}

func bundleByID(id int) *Bundle {
	for _, b := range Bundles {
		if b.ID == id {
			return &b
		}
	}
	return nil
}

func totalCoins(b *Bundle) int {
	return b.Coins + (b.Coins * b.BonusPct / 100)
}

type FundHandler struct {
	DB *db.Database
}

func (h *FundHandler) RegisterRoutes(r chi.Router) {
	r.Get("/fund/bundles", h.GetBundles)
	r.Post("/fund/stripe/create-session", h.StripeCreateSession)
	r.Post("/fund/solana/verify", h.SolanaVerify)
}

// GetBundles returns the available coin bundles
func (h *FundHandler) GetBundles(w http.ResponseWriter, r *http.Request) {
	type BundleResponse struct {
		ID         int    `json:"id"`
		Coins      int    `json:"coins"`
		BonusPct   int    `json:"bonusPct"`
		TotalCoins int    `json:"totalCoins"`
		PriceCents int    `json:"priceCents"`
		Label      string `json:"label"`
	}
	var resp []BundleResponse
	for _, b := range Bundles {
		resp = append(resp, BundleResponse{
			ID:         b.ID,
			Coins:      b.Coins,
			BonusPct:   b.BonusPct,
			TotalCoins: totalCoins(&b),
			PriceCents: b.PriceCents,
			Label:      b.Label,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// --- Stripe Flow ---

type StripeSessionRequest struct {
	BundleID int `json:"bundleId"`
}

func (h *FundHandler) StripeCreateSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	var req StripeSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	bundle := bundleByID(req.BundleID)
	if bundle == nil {
		http.Error(w, "Invalid bundle", http.StatusBadRequest)
		return
	}

	stripe.Key = config.AppConfig.StripeKey

	coins := totalCoins(bundle)

	// Create a pending transaction
	var txID int
	err := h.DB.Pool.QueryRow(r.Context(),
		"INSERT INTO transactions (user_id, provider, amount_coins, amount_fiat_cents, status) VALUES ($1, 'stripe', $2, $3, 'pending') RETURNING id",
		userID, coins, bundle.PriceCents,
	).Scan(&txID)
	if err != nil {
		log.Printf("Failed to create transaction record: %v", err)
		http.Error(w, "Failed to create transaction", http.StatusInternalServerError)
		return
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = "http://localhost:21050"
	}

	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency:   stripe.String("usd"),
					UnitAmount: stripe.Int64(int64(bundle.PriceCents)),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name:        stripe.String(fmt.Sprintf("%s GUESS Coins", bundle.Label)),
						Description: stripe.String(fmt.Sprintf("%d GUESS Coins for gameplay", coins)),
					},
				},
				Quantity: stripe.Int64(1),
			},
		},
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(origin + "/buy-coins?status=success&session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:  stripe.String(origin + "/buy-coins?status=cancelled"),
		Metadata: map[string]string{
			"user_id":        strconv.Itoa(userID),
			"bundle_id":      strconv.Itoa(bundle.ID),
			"transaction_id": strconv.Itoa(txID),
			"coins":          strconv.Itoa(coins),
		},
	}

	s, err := session.New(params)
	if err != nil {
		log.Printf("Stripe session creation failed: %v", err)
		// Mark transaction failed
		h.DB.Pool.Exec(r.Context(), "UPDATE transactions SET status = 'failed' WHERE id = $1", txID)
		http.Error(w, "Failed to create checkout session", http.StatusInternalServerError)
		return
	}

	// Store Stripe session ID on the transaction
	h.DB.Pool.Exec(r.Context(), "UPDATE transactions SET provider_id = $1 WHERE id = $2", s.ID, txID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"sessionId":  s.ID,
		"sessionUrl": s.URL,
	})
}

// StripeWebhook handles Stripe webhook events (called outside auth middleware)
func (h *FundHandler) StripeWebhook(w http.ResponseWriter, r *http.Request) {
	const maxBodyBytes = int64(65536)
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "Error reading body", http.StatusServiceUnavailable)
		return
	}

	sigHeader := r.Header.Get("Stripe-Signature")
	endpointSecret := config.AppConfig.StripeWebhookSecret

	var event stripe.Event
	if endpointSecret != "" {
		event, err = webhook.ConstructEvent(body, sigHeader, endpointSecret)
		if err != nil {
			log.Printf("Stripe webhook signature verification failed: %v", err)
			http.Error(w, "Invalid signature", http.StatusBadRequest)
			return
		}
	} else {
		// Dev mode: no webhook secret, parse raw event
		if err := json.Unmarshal(body, &event); err != nil {
			http.Error(w, "Invalid event", http.StatusBadRequest)
			return
		}
		log.Println("[WARN] Stripe webhook secret not configured — skipping signature verification")
	}

	if event.Type == "checkout.session.completed" {
		var sess stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
			log.Printf("Stripe webhook unmarshal error: %v", err)
			http.Error(w, "Parse error", http.StatusBadRequest)
			return
		}

		txIDStr := sess.Metadata["transaction_id"]
		userIDStr := sess.Metadata["user_id"]
		coinsStr := sess.Metadata["coins"]
		txID, _ := strconv.Atoi(txIDStr)
		userID, _ := strconv.Atoi(userIDStr)
		coins, _ := strconv.Atoi(coinsStr)

		if txID == 0 || userID == 0 || coins == 0 {
			log.Printf("Stripe webhook missing metadata: tx=%s user=%s coins=%s", txIDStr, userIDStr, coinsStr)
			w.WriteHeader(http.StatusOK)
			return
		}

		// Check if already completed (idempotency)
		var status string
		h.DB.Pool.QueryRow(r.Context(), "SELECT status FROM transactions WHERE id = $1", txID).Scan(&status)
		if status == "completed" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Credit coins and mark transaction complete
		h.DB.Pool.Exec(r.Context(), "UPDATE users SET guess_coins = guess_coins + $1 WHERE id = $2", coins, userID)
		h.DB.Pool.Exec(r.Context(), "UPDATE transactions SET status = 'completed', provider_id = $1 WHERE id = $2", sess.ID, txID)
		log.Printf("Stripe: credited %d coins to user %d (tx %d)", coins, userID, txID)
	}

	w.WriteHeader(http.StatusOK)
}

// --- Solana Flow ---

type SolanaVerifyRequest struct {
	BundleID    int    `json:"bundleId"`
	TxSignature string `json:"txSignature"`
}

func (h *FundHandler) SolanaVerify(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	var req SolanaVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	bundle := bundleByID(req.BundleID)
	if bundle == nil {
		http.Error(w, "Invalid bundle", http.StatusBadRequest)
		return
	}

	if req.TxSignature == "" {
		http.Error(w, "Transaction signature required", http.StatusBadRequest)
		return
	}

	// Check idempotency — has this signature already been used?
	var existingCount int
	h.DB.Pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM transactions WHERE provider = 'solana' AND provider_id = $1 AND status = 'completed'", req.TxSignature).Scan(&existingCount)
	if existingCount > 0 {
		http.Error(w, "Transaction already processed", http.StatusConflict)
		return
	}

	coins := totalCoins(bundle)

	// Create transaction record
	var txID int
	err := h.DB.Pool.QueryRow(r.Context(),
		"INSERT INTO transactions (user_id, provider, provider_id, amount_coins, amount_fiat_cents, status) VALUES ($1, 'solana', $2, $3, $4, 'pending') RETURNING id",
		userID, req.TxSignature, coins, bundle.PriceCents,
	).Scan(&txID)
	if err != nil {
		log.Printf("Failed to create solana transaction record: %v", err)
		http.Error(w, "Failed to create transaction", http.StatusInternalServerError)
		return
	}

	// TODO: In production, verify the transaction on-chain:
	// 1. Fetch tx from Solana RPC using req.TxSignature
	// 2. Verify it transfers the correct SOL amount to our receiver wallet
	// 3. Verify it is finalized/confirmed
	// For now, we trust the client (dev mode)
	verified := true

	if !verified {
		h.DB.Pool.Exec(r.Context(), "UPDATE transactions SET status = 'failed' WHERE id = $1", txID)
		http.Error(w, "Transaction verification failed", http.StatusBadRequest)
		return
	}

	// Credit coins
	h.DB.Pool.Exec(r.Context(), "UPDATE users SET guess_coins = guess_coins + $1 WHERE id = $2", coins, userID)
	h.DB.Pool.Exec(r.Context(), "UPDATE transactions SET status = 'completed' WHERE id = $1", txID)
	log.Printf("Solana: credited %d coins to user %d (tx %d, sig %s)", coins, userID, txID, req.TxSignature)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"coinsAdded":    coins,
		"transactionId": txID,
	})
}
