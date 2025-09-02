package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"user-service-new/internal/domain/entities"
)

type RedisService struct {
	client *redis.Client
}

func NewRedisService() *RedisService {
	// Get Redis configuration from environment variables
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost"
	}

	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}

	password := os.Getenv("REDIS_PASSWORD")
	if password == "" {
		password = ""
	}

	db := GetEnvAsInt("REDIS_DB", 0)

	// Alternative: Use REDIS_URL if provided
	redisURL := os.Getenv("REDIS_URL")
	if redisURL != "" {
		opt, err := redis.ParseURL(redisURL)
		if err == nil {
			client := redis.NewClient(opt)
			// Test connection
			ctx := context.Background()
			if err := client.Ping(ctx).Err(); err != nil {
				fmt.Printf("Warning: Redis connection failed with REDIS_URL: %v\n", err)
			} else {
				fmt.Printf("Connected to Redis using REDIS_URL: %s\n", redisURL)
				return &RedisService{client: client}
			}
		}
	}

	// Use individual environment variables
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", host, port),
		Password: password,
		DB:       db,
	})

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		fmt.Printf("Warning: Redis connection failed: %v\n", err)
		fmt.Printf("Redis will be disabled. Some features may not work properly.\n")
		// Return a mock Redis service that doesn't fail
		return &RedisService{client: nil}
	}

	fmt.Printf("Connected to Redis at %s:%s\n", host, port)
	return &RedisService{
		client: client,
	}
}

func (r *RedisService) SetToken(ctx context.Context, token, userID string, ttl time.Duration) error {
	if r.client == nil {
		return nil // Redis disabled
	}
	return r.client.Set(ctx, "token:"+token, userID, ttl).Err()
}

func (r *RedisService) GetToken(ctx context.Context, token string) (string, error) {
	if r.client == nil {
		return "", redis.Nil // Redis disabled, return nil as if key doesn't exist
	}
	result, err := r.client.Get(ctx, "token:"+token).Result()
	if err != nil {
		return "", err
	}
	return result, nil
}

func (r *RedisService) SetOTP(ctx context.Context, key, otp string, ttl time.Duration) error {
	if r.client == nil {
		return nil // Redis disabled
	}
	return r.client.Set(ctx, key, otp, ttl).Err()
}

func (r *RedisService) GetOTP(ctx context.Context, key string) (string, error) {
	if r.client == nil {
		return "", redis.Nil // Redis disabled, return nil as if key doesn't exist
	}
	return r.client.Get(ctx, key).Result()
}

func (r *RedisService) SetUserData(ctx context.Context, email string, user *entities.User, ttl time.Duration) error {
	if r.client == nil {
		return nil // Redis disabled
	}
	userData, err := json.Marshal(user)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, "user:"+email, userData, ttl).Err()
}

func (r *RedisService) GetUserData(ctx context.Context, email string) (*entities.User, error) {
	if r.client == nil {
		return nil, redis.Nil // Redis disabled, return nil as if key doesn't exist
	}
	userData, err := r.client.Get(ctx, "user:"+email).Result()
	if err != nil {
		return nil, err
	}

	var user entities.User
	if err := json.Unmarshal([]byte(userData), &user); err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *RedisService) SetProfile(ctx context.Context, userID string, user *entities.User, ttl time.Duration) error {
	if r.client == nil {
		return nil // Redis disabled
	}
	userData, err := json.Marshal(user)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, "profile:"+userID, userData, ttl).Err()
}

func (r *RedisService) GetProfile(ctx context.Context, userID string) (*entities.User, error) {
	if r.client == nil {
		return nil, redis.Nil // Redis disabled, return nil as if key doesn't exist
	}
	userData, err := r.client.Get(ctx, "profile:"+userID).Result()
	if err != nil {
		return nil, err
	}

	var user entities.User
	if err := json.Unmarshal([]byte(userData), &user); err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *RedisService) DeleteKey(ctx context.Context, key string) error {
	if r.client == nil {
		return nil // Redis disabled
	}
	return r.client.Del(ctx, key).Err()
}

func (r *RedisService) Close() error {
	if r.client == nil {
		return nil // Redis disabled
	}
	return r.client.Close()
}
