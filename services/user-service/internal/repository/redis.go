package repository

import (
    "context"
    "time"
    "github.com/redis/go-redis/v9"
)

type RedisRepo struct {
    client *redis.Client
}

func NewRedisRepo(client *redis.Client) *RedisRepo {
    return &RedisRepo{client: client}
}

func (r *RedisRepo) SetToken(ctx context.Context, key string, value string, expiration time.Duration) error {
    return r.client.Set(ctx, key, value, expiration).Err()
}

func (r *RedisRepo) GetToken(ctx context.Context, key string) (string, error) {
    return r.client.Get(ctx, key).Result()
}
