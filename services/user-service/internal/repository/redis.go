package repository

import (
    "context"
    "time"
    "github.com/redis/go-redis/v9"
    "user-service/internal/domain"
    "encoding/json"
)

type RedisRepo struct {
    client *redis.Client
}

func NewRedisRepo(client *redis.Client) *RedisRepo {
    return &RedisRepo{client: client}
}

func (r *RedisRepo) DeleteKey(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *RedisRepo) SetToken(ctx context.Context, key string, value string, expiration time.Duration) error {
    return r.client.Set(ctx, key, value, expiration).Err()
}

func (r *RedisRepo) GetToken(ctx context.Context, key string) (string, error) {
    return r.client.Get(ctx, key).Result()
}

func (r *RedisRepo) SetProfile(ctx context.Context, userID string, user *domain.User, expiration time.Duration) error {
    // Convert user to JSON string for storage in Redis
    data, err := json.Marshal(user)
    if err != nil {
        return err
    }

    // Store the serialized user data in Redis with the specified expiration time
    return r.client.Set(ctx, userID, data, expiration).Err()
}

func (r *RedisRepo) GetProfile(ctx context.Context, userID string) (*domain.User, error) {
    var user domain.User
    data, err := r.client.Get(ctx, userID).Result()
    if err != nil {
        if err == redis.Nil {
            return nil, nil // No profile found in cache
        }
        return nil, err // Return error if there's a different issue
    }

    // Deserialize the user data from JSON
    err = json.Unmarshal([]byte(data), &user)
    if err != nil {
        return nil, err
    }

    return &user, nil
}

// SetOTP stores the OTP for a given email with expiration
func (r *RedisRepo) SetOTP(ctx context.Context, email string, otp string, expiration time.Duration) error {
	
	return r.client.Set(ctx, "otp:"+email, otp, expiration).Err()
}

// GetOTP retrieves the OTP for a given email
func (r *RedisRepo) GetOTP(ctx context.Context, email string) (string, error) {
	data, err := r.client.Get(ctx, "otp:"+email).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}
	
	return data, nil
}