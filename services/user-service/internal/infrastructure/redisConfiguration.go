package infrastructure

import (
    "fmt"
    "os"
    "time"

    "github.com/redis/go-redis/v9"
)

func NewRedisClient() *redis.Client {
    host := os.Getenv("REDIS_HOST")
    port := os.Getenv("REDIS_PORT")
    password := os.Getenv("REDIS_PASSWORD")
    // username is not needed for most Redis clients; if needed, you can handle separately

    if host == "" || port == "" {
        panic("REDIS_HOST or REDIS_PORT env variables are missing")
    }

    addr := fmt.Sprintf("%s:%s", host, port)

    return redis.NewClient(&redis.Options{
        Addr:         addr,
        Password:     password,
        DB:           0,
        PoolSize:     10,
        MinIdleConns: 5,
        DialTimeout:  5 * time.Second,
        ReadTimeout:  3 * time.Second,
        WriteTimeout: 3 * time.Second,
    })
}
