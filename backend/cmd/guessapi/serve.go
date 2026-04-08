package guesscmd

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"guessapi/internal/api"
	"guessapi/internal/config"
	"guessapi/internal/db"

	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts the HTTP server",
	Run: func(cmd *cobra.Command, args []string) {
		dbUrl := config.AppConfig.DatabaseURL
		if dbUrl == "" {
			dbUrl = "postgres://postgres:postgres@localhost:5432/guessdb?sslmode=disable"
		}

		// Connect to DB and run migrations
		database, err := db.Connect(context.Background(), dbUrl)
		if err != nil {
			log.Printf("DB Connect Warning: %v (skipping for now)", err)
		} else {
			defer database.Pool.Close()
			err = db.RunMigrations(dbUrl, "internal/migrations")
			if err != nil {
				log.Fatalf("Migration failed: %v", err)
			}
		}
		router := api.NewRouter(database)

		serverAddr := fmt.Sprintf(":%s", config.AppConfig.Port)
		fmt.Printf("Server starting on %s...\n", serverAddr)
		if err := http.ListenAndServe(serverAddr, router); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	},
}

func init() {
	// Add flags here if needed e.g. port
}
