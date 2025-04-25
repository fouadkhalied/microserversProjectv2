package config

import "os"

type Config struct {
    PostgreSQL string
}

func Load() *Config {
    return &Config{
        PostgreSQL: os.Getenv("PostgreSQL"),
    }
}