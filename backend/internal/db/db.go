package db

import (
    "context"
    "fmt"
    "log"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/postgres"
    _ "github.com/golang-migrate/migrate/v4/source/file"
)

type Database struct {
    Pool *pgxpool.Pool
}

func Connect(ctx context.Context, connectionString string) (*Database, error) {
    pool, err := pgxpool.New(ctx, connectionString)
    if err != nil {
        return nil, fmt.Errorf("unable to connect to database: %w", err)
    }

    if err := pool.Ping(ctx); err != nil {
        return nil, fmt.Errorf("database not reachable: %w", err)
    }

    return &Database{Pool: pool}, nil
}

func RunMigrations(connectionString string, migrationsPath string) error {
    log.Println("Running database migrations...")
    m, err := migrate.New(
        fmt.Sprintf("file://%s", migrationsPath),
        connectionString,
    )
    if err != nil {
        return fmt.Errorf("could not create migrate instance: %w", err)
    }

    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return fmt.Errorf("migration failed: %w", err)
    }

    log.Println("Database migrations standard check completed (up to date).")
    return nil
}
