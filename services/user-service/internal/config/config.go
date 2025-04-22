package config

import "os"

type Config struct {
    MongoURI string
}

func Load() *Config {
    return &Config{
        MongoURI: os.Getenv("MongoURI"),
    }
}
