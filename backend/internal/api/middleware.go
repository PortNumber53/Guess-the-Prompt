package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/time/rate"
)

var JWTSecret = []byte("super-secret-key-change-in-production")

type ContextKey string

const UserIDKey ContextKey = "user_id"

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// If not authenticated, we just proceed (could be a public guest)
			next.ServeHTTP(w, r)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
			return
		}

		tokenStr := parts[1]
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			return JWTSecret, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			userID := int(claims["user_id"].(float64))
			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		http.Error(w, "Invalid token payload", http.StatusUnauthorized)
	})
}

// Rate Limiter logic
var (
	limiters   = make(map[string]*rate.Limiter)
	limitMutex sync.Mutex
)

func getVisitor(id string) *rate.Limiter {
	limitMutex.Lock()
	defer limitMutex.Unlock()

	limiter, exists := limiters[id]
	if !exists {
		// 30 requests per minute = 0.5 req / sec, burst size of 30.
		limiter = rate.NewLimiter(rate.Limit(0.5), 30)
		limiters[id] = limiter
	}
	return limiter
}

func RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Attempt to grab authenticated user
		identifier := ""
		userID := r.Context().Value(UserIDKey)
		if userID != nil {
			identifier = "user_" + strconv.Itoa(userID.(int))
		} else {
			// Fallback to IP address
			identifier = "ip_" + strings.Split(r.RemoteAddr, ":")[0]
		}

		limiter := getVisitor(identifier)
		if !limiter.Allow() {
			http.Error(w, "Rate limit exceeded (30 requests per minute)", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
