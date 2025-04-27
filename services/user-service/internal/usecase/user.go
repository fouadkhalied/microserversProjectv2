
package usecase

import (
    "context"
    "log"
    "time"
    "user-service/internal/domain"
    "user-service/internal/infastructure"
    "user-service/internal/repository"
    "golang.org/x/crypto/bcrypt"
)

type UserUsecase struct {
    userRepo   *repository.UserRepo
    redisRepo  *repository.RedisRepo
    jwtService *infastructure.JWTService
    // Added cache to avoid frequent database lookups
    userCache  map[string]*domain.User
    cacheTTL   time.Duration
}

func NewUserUsecase(userRepo *repository.UserRepo, redisRepo *repository.RedisRepo, jwtService *infastructure.JWTService) *UserUsecase {
    return &UserUsecase{
        userRepo:   userRepo,
        redisRepo:  redisRepo,
        jwtService: jwtService,
        userCache:  make(map[string]*domain.User),
        cacheTTL:   5 * time.Minute, // Cache users for 5 minutes
    }
}


func (uc *UserUsecase) RegisterUser(ctx context.Context, user *domain.User) error {
    
    // Use a lower cost factor for bcrypt during registration
    // This significantly reduces CPU time while maintaining decent security
    // DefaultCost is 10, we can use 8 for faster performance
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