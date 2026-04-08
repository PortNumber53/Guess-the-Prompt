package config

import (
    "log"
    "os"
    "path/filepath"

    "github.com/spf13/viper"
)

type Config struct {
    DatabaseURL string `mapstructure:"DATABASE_URL"`
    StripeKey   string `mapstructure:"STRIPE_KEY"`
    Port        string `mapstructure:"PORT"`
}

var AppConfig Config

func LoadConfig() {
    viper.SetConfigName("config")
    viper.SetConfigType("env")

    // Add /etc/ path
    viper.AddConfigPath("/etc/api-guess-the-prompt/")

    // Add ~/.config/ path
    home, err := os.UserHomeDir()
    if err == nil {
        viper.AddConfigPath(filepath.Join(home, ".config", "guess-the-prompt"))
    }

    // Also look in current directory for development convenience
    viper.AddConfigPath(".")

    // Read config
    if err := viper.ReadInConfig(); err != nil {
        if _, ok := err.(viper.ConfigFileNotFoundError); ok {
            log.Println("No config.ini file found, falling back to environment variables")
        } else {
            log.Printf("Error reading config file: %v\n", err)
        }
    } else {
        log.Printf("Using config file: %s\n", viper.ConfigFileUsed())
    }

    // Match ENV variables
    viper.AutomaticEnv()

    // Unmarshal to struct
    if err := viper.Unmarshal(&AppConfig); err != nil {
        log.Fatalf("Unable to decode into struct: %v", err)
    }

    // Set defaults if empty
    if AppConfig.Port == "" {
        AppConfig.Port = "8080"
    }
}
