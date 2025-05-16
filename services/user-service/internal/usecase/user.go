package usecase

import (
	"context"
	"errors"
	"log"
	"time"
	"user-service/internal/domain"
	"user-service/internal/infrastructure"
	"user-service/internal/repository"
    "fmt"
	"golang.org/x/crypto/bcrypt"
)

type UserUsecase struct {
    userRepo   *repository.UserRepo
    redisRepo  *repository.RedisRepo
    jwtService *infrastructure.JWTService
    otpService *infrastructure.OTPService
    otpRateLimiter *infrastructure.RateLimiter
    userCache  map[string]*domain.User
    cacheTTL   time.Duration
}

func NewUserUsecase(userRepo *repository.UserRepo, redisRepo *repository.RedisRepo, jwtService *infrastructure.JWTService, otpService *infrastructure.OTPService, otpRateLimiter *infrastructure.RateLimiter) *UserUsecase {
    
    // Start periodic cleanup of stale rate limiter entries
    go func() {
        ticker := time.NewTicker(1 * time.Hour)
        defer ticker.Stop()
        
        for range ticker.C {
            otpRateLimiter.CleanupStaleEntries()
        }
    }()
    return &UserUsecase{
        userRepo:   userRepo,
        redisRepo:  redisRepo,
        jwtService: jwtService,
        otpService: otpService,
        otpRateLimiter: otpRateLimiter,
        userCache:  make(map[string]*domain.User),
        cacheTTL:   5 * time.Minute, // Cache users for 5 minutes
    }
}

func (uc *UserUsecase) RegisterUser(ctx context.Context, user *domain.User) error {
    
    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), 8)
    if err != nil {
        return err
    }
    user.Password = string(hashedPassword)
    
    // Initialize tokens array if nil
    if user.Tokens == nil {
        user.Tokens = make([]string, 0)
    }
    
    // Use context with timeout for the database operation
    dbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()
    
    return uc.userRepo.CreateUser(dbCtx, user)
}

func (uc *UserUsecase) LoginUser(ctx context.Context, username, password string) (string, error) {
    // Start multiple operations concurrently
    type userResult struct {
        user *domain.User
        err  error
    }
    
    userCh := make(chan userResult, 1)
    
    // Try to get user from cache first
    cachedUser, found := uc.userCache[username]
    
    if found {
        // Use the cached user
        userCh <- userResult{user: cachedUser, err: nil}
    } else {
        // Fetch user from database if not in cache
        go func() {
            user, err := uc.userRepo.FindByCredentials(ctx, username)
            if err == nil {
                // Cache the user for future requests
                uc.userCache[username] = user
                // Setup cache expiration (simplified implementation)
                go func(uname string) {
                    time.Sleep(uc.cacheTTL)
                    delete(uc.userCache, uname)
                }(username)
            }
            userCh <- userResult{user: user, err: err}
        }()
    }
    
    // Wait for user retrieval
    result := <-userCh
    if result.err != nil {
        return "", result.err
    }
    
    // Verify password
    err := bcrypt.CompareHashAndPassword([]byte(result.user.Password), []byte(password))
    if err != nil {
        return "", err
    }
    
    // Generate JWT token asynchronously
    tokenCh := make(chan string, 1)
    errCh := make(chan error, 1)
    
    go func() {
        token, err := uc.jwtService.GenerateToken(result.user.ID)
        if err != nil {
            errCh <- err
            return
        }
        tokenCh <- token
    }()
    
    // Wait for token generation
    select {
    case err := <-errCh:
        return "", err
    case token := <-tokenCh:
        // Store token in Redis and update database concurrently
        go func() {
            // Store in Redis for quick validation
            redisErr := uc.redisRepo.SetToken(context.Background(), token, result.user.ID, time.Hour*24)
            if redisErr != nil {
                log.Printf("Failed to store token in Redis: %v", redisErr)
            }
            
            // Update user's tokens in PostgreSQL asynchronously
            // This doesn't need to block the login response
            dbErr := uc.userRepo.UpdateTokens(context.Background(), result.user.ID, token)
            if dbErr != nil {
                log.Printf("Failed to update tokens in database: %v", dbErr)
            }
        }()
        
        return token, nil
    case <-ctx.Done():
        return "", ctx.Err()
    }
}


func (uc *UserUsecase) GetProfile(ctx context.Context, userID string) (*domain.User, error) {
    // First, try to get the profile from Redis cache
    cachedUser, err := uc.redisRepo.GetProfile(ctx, userID)
    if err == nil && cachedUser != nil {
        // Cache hit, return the cached profile (exclude password)
        cachedUser.Password = ""
        return cachedUser, nil
    }

    // If not in cache, get it from the database
    user, err := uc.userRepo.GetProfile(ctx, userID)
    if err != nil {
        return nil, err
    }

    // Cache the user profile in Redis for future access, with TTL
    err = uc.redisRepo.SetProfile(ctx, userID, user, 24*time.Hour) // Cache for 24 hours
    if err != nil {
        log.Printf("Failed to cache user profile: %v", err)
    }
    return user, nil
}
func (uc *UserUsecase) SendOTPtoUser(ctx context.Context, user *domain.User) error {
    // Check if user already exists in database
    existingUser, err := uc.userRepo.FindByCredentials(ctx, user.Username)
    // if err != nil {
    //     // Only return unexpected errors, not "user not found" which is expected
    //     return fmt.Errorf("error checking existing user: %w", err)
    // }

    if existingUser != nil {
        return errors.New("username already exists")
    }
    
    // Apply rate limiting for OTP generation
    // if !uc.otpRateLimiter.Allow(user.Email) {
    //     return errors.New("too many OTP requests, please try again later")
    // }

    // consistent key naming
    otpKey := "otp:" + user.Email
    
    // Check if OTP already exists in cache and hasn't expired
    otp, err := uc.redisRepo.GetOTP(ctx, otpKey)
    if err != nil {
        return fmt.Errorf("redis error: %w", err)
    }

    // Generate new OTP if needed
    if otp == "" {
        otp = uc.otpService.GenerateOTP(ctx)
        
        // Set OTP in cache with 5-minute expiration
        if err := uc.redisRepo.SetOTP(ctx, otpKey, otp, 5*time.Minute); err != nil {
            return fmt.Errorf("failed to cache OTP: %w", err)
        }
    }

    // Send OTP to user
    if err := uc.otpService.SendOTP(ctx, user.Email, otp); err != nil {
        // Clean up the cached OTP if we couldn't send it
        uc.redisRepo.DeleteKey(ctx, otpKey)
        return fmt.Errorf("failed to send OTP: %w", err)
    }

    // Store user data with a longer TTL (15 minutes)
    if err := uc.redisRepo.SetUserData(ctx, user.Email, user, 15*time.Minute); err != nil {
        return fmt.Errorf("failed to cache user data: %w", err)
    }

    return nil
}
func (uc *UserUsecase) VerifyOtp(ctx context.Context, email, userOtp string) error {
    // Apply rate limiting for OTP verification attempts
    if !uc.otpRateLimiter.Allow("verify:" + email) {
        return errors.New("too many verification attempts, please try again later")
    }

    // Use consistent key naming
    otpKey := "otp:" + email

    // Get OTP from cache
    cacheOtp, err := uc.redisRepo.GetOTP(ctx, otpKey)
    if err != nil {
        return fmt.Errorf("failed to retrieve OTP from cache: %w", err)
    }
    
    // Check if OTP exists
    if cacheOtp == "" {
        return errors.New("OTP expired or not found")
    }
    
    // Don't log sensitive information like OTPs
    // log.Printf(email, userOtp, cacheOtp) - REMOVE THIS LINE
    
    // Verify OTP
    isValid, err := uc.otpService.VerifyOTP(ctx, email, userOtp, cacheOtp)
    if err != nil {
        return fmt.Errorf("OTP verification failed: %w", err)
    }

    if !isValid {
        return errors.New("invalid OTP")
    }
    
    // If OTP is valid, get user data from cache
    user, err := uc.redisRepo.GetUserData(ctx, email)
    if err != nil {
        return fmt.Errorf("failed to retrieve user data: %w", err)
    }
    
    if user == nil {
        return errors.New("user data expired or not found")
    }

    // Register the user
    if err := uc.RegisterUser(ctx, user); err != nil {
        return fmt.Errorf("failed to register user: %w", err)
    }
    
    // Clean up cache after successful registration
    if err := uc.redisRepo.DeleteKey(ctx, otpKey); err != nil {
        // Just log this error, don't return it as the registration was successful
        log.Printf("Warning: Failed to delete OTP key: %v", err)
    }
    
    // Also clean up the user data cache
    if err := uc.redisRepo.DeleteKey(ctx, "user:"+email); err != nil {
        log.Printf("Warning: Failed to delete user data cache: %v", err)
    }

    return nil
}