package usecase

import (
	"context"
	"log"
	"time"
	"user-service/internal/domain"
	"user-service/internal/infrastructure"
	"user-service/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

type UserUsecase struct {
    userRepo   *repository.UserRepo
    redisRepo  *repository.RedisRepo
    jwtService *infrastructure.JWTService
    otpService *infrastructure.OTPService
    userCache  map[string]*domain.User
    cacheTTL   time.Duration
}

func NewUserUsecase(userRepo *repository.UserRepo, redisRepo *repository.RedisRepo, jwtService *infrastructure.JWTService, otpService *infrastructure.OTPService) *UserUsecase {
    return &UserUsecase{
        userRepo:   userRepo,
        redisRepo:  redisRepo,
        jwtService: jwtService,
        otpService: otpService,
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

func (uc *UserUsecase) SendOTPtoUser(ctx context.Context, user *domain.User) (error) {
    // Check if OTP already exists in cache and hasn't expired
    otp, err := uc.redisRepo.GetOTP(ctx, user.Email)
    if err != nil {
        return err // Redis error
    }

    if otp != "" {
        return nil // OTP still valid; no need to resend
    } else {
        otp = uc.otpService.GenerateOTP(ctx) // generate new otp
        uc.redisRepo.SetOTP(ctx,"otp:"+user.Email,otp,time.Minute * 1) // set otp in cache
    } 

    // Send new OTP
    if err := uc.otpService.SendOTP(ctx,user.Email,otp); err != nil {
        return err // Failed to send OTP
    }

    // Store user data with otp in cache
    uc.redisRepo.SetUserData(ctx,user.Email,user,time.Second * 50)

    return nil // OTP sent successfully
}

func (uc *UserUsecase) VerifyOtp(ctx context.Context,email, userOtp string) (error) {

    // get otp from cache

    cacheOtp,err := uc.redisRepo.GetOTP(ctx,"otp:"+email)

    if err != nil {
        log.Printf("Failed to retrive otp from cache : %v", err)
        return err
    }
    log.Printf(email , userOtp , cacheOtp)
    isValid , err := uc.otpService.VerifyOTP(ctx,email,userOtp,cacheOtp)

    if err != nil {
        log.Printf("Failed to verify user : %v", err)
        return err
    }

    if isValid {
        user , err := uc.redisRepo.GetUserData(ctx,email)

        if err != nil {
            log.Printf("Failed to retrive user from cache : %v", err)
            return err
        } 

        log.Print(user)
        uc.RegisterUser(ctx,user)
    }
    return nil
}