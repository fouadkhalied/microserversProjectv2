package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"user-service-new/internal/application/command"
	"user-service-new/internal/application/interfaces"
	"user-service-new/internal/application/mapper"
	"user-service-new/internal/application/query"
	"user-service-new/internal/domain/entities"
	"user-service-new/internal/domain/repositories"
	"user-service-new/internal/infrastructure"
)

type UserService struct {
	userRepo        repositories.UserRepository
	idempotencyRepo repositories.IdempotencyRepository
	redisService    *infrastructure.RedisService
	jwtService      *infrastructure.JWTService
	otpService      *infrastructure.OTPService
	rateLimiter     *infrastructure.RateLimiter
}

func NewUserService(
	userRepo repositories.UserRepository,
	idempotencyRepo repositories.IdempotencyRepository,
	redisService *infrastructure.RedisService,
	jwtService *infrastructure.JWTService,
	otpService *infrastructure.OTPService,
	rateLimiter *infrastructure.RateLimiter,
) interfaces.UserService {
	return &UserService{
		userRepo:        userRepo,
		idempotencyRepo: idempotencyRepo,
		redisService:    redisService,
		jwtService:      jwtService,
		otpService:      otpService,
		rateLimiter:     rateLimiter,
	}
}

func (s *UserService) CreateUser(createCommand *command.CreateUserCommand) (*command.CreateUserCommandResult, error) {
	ctx := context.Background()

	// Check idempotency key
	if createCommand.IdempotencyKey != "" {
		existingRecord, err := s.idempotencyRepo.FindByKey(ctx, createCommand.IdempotencyKey)
		if err != nil {
			return nil, err
		}

		if existingRecord != nil {
			// Return cached response
			var result command.CreateUserCommandResult
			if err := json.Unmarshal([]byte(existingRecord.Response), &result); err != nil {
				return nil, err
			}
			return &result, nil
		}
	}

	// Create idempotency record
	var idempotencyRecord *entities.IdempotencyRecord
	if createCommand.IdempotencyKey != "" {
		requestJSON, _ := json.Marshal(createCommand)
		idempotencyRecord = entities.NewIdempotencyRecord(createCommand.IdempotencyKey, string(requestJSON))
	}

	// Check if user already exists
	existingUser, err := s.userRepo.FindByUsername(createCommand.Username)
	if err != nil {
		return nil, err
	}
	if existingUser != nil {
		return nil, errors.New("username already exists")
	}

	existingUser, err = s.userRepo.FindByEmail(createCommand.Email)
	if err != nil {
		return nil, err
	}
	if existingUser != nil {
		return nil, errors.New("email already exists")
	}

	// Create new user
	newUser := entities.NewUser(createCommand.Username, createCommand.Email, createCommand.Password)
	validatedUser, err := entities.NewValidatedUser(newUser)
	if err != nil {
		return nil, err
	}

	createdUser, err := s.userRepo.Create(validatedUser)
	if err != nil {
		return nil, err
	}

	result := command.CreateUserCommandResult{
		Result: mapper.NewUserResultFromEntity(createdUser),
	}

	// Store response in idempotency record
	if idempotencyRecord != nil {
		responseJSON, _ := json.Marshal(result)
		idempotencyRecord.SetResponse(string(responseJSON), 200)
		_, err = s.idempotencyRepo.Create(ctx, idempotencyRecord)
		if err != nil {
			log.Printf("Failed to store idempotency record: %v", err)
		}
	}

	return &result, nil
}

func (s *UserService) LoginUser(loginCommand *command.LoginUserCommand) (*command.LoginUserCommandResult, error) {

	// Find user by credentials
	user, err := s.userRepo.FindByCredentials(loginCommand.Username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("invalid credentials")
	}

	// Check password
	if err := user.CheckPassword(loginCommand.Password); err != nil {
		return nil, errors.New("invalid credentials")
	}

	// Generate JWT token
	token, err := s.jwtService.GenerateToken(user.Id.String())
	if err != nil {
		return nil, err
	}

	// Store token in Redis and update database concurrently
	go func() {
		// Store in Redis for quick validation
		redisErr := s.redisService.SetToken(context.Background(), token, user.Id.String(), time.Hour*24)
		if redisErr != nil {
			log.Printf("Failed to store token in Redis: %v", redisErr)
		}

		// Update user's tokens in PostgreSQL asynchronously
		dbErr := s.userRepo.UpdateTokens(context.Background(), user.Id, token)
		if dbErr != nil {
			log.Printf("Failed to update tokens in database: %v", dbErr)
		}
	}()

	result := command.LoginUserCommandResult{
		Token: token,
		User:  mapper.NewUserResultFromEntity(user),
	}

	return &result, nil
}

func (s *UserService) SendOTP(sendOTPCommand *command.SendOTPCommand) (*command.SendOTPCommandResult, error) {
	ctx := context.Background()

	// Check idempotency key
	if sendOTPCommand.IdempotencyKey != "" {
		existingRecord, err := s.idempotencyRepo.FindByKey(ctx, sendOTPCommand.IdempotencyKey)
		if err != nil {
			return nil, err
		}

		if existingRecord != nil {
			// Return cached response
			var result command.SendOTPCommandResult
			if err := json.Unmarshal([]byte(existingRecord.Response), &result); err != nil {
				return nil, err
			}
			return &result, nil
		}
	}

	// Check if user already exists
	existingUser, err := s.userRepo.FindByUsername(sendOTPCommand.Username)
	if err != nil {
		return nil, err
	}
	if existingUser != nil {
		return nil, errors.New("username already exists")
	}

	// Apply rate limiting for OTP generation
	if !s.rateLimiter.Allow(sendOTPCommand.Email) {
		return nil, errors.New("too many OTP requests, please try again later")
	}

	// Check if OTP already exists in cache and hasn't expired
	otpKey := "otp:" + sendOTPCommand.Email
	otp, err := s.redisService.GetOTP(ctx, otpKey)
	if err != nil {
		// If Redis is not available or key doesn't exist, continue with new OTP generation
		if err.Error() == "redis: nil" {
			otp = ""
		} else {
			return nil, fmt.Errorf("redis error: %w", err)
		}
	}

	// Generate new OTP if needed
	if otp == "" {
		otp = s.otpService.GenerateOTP(ctx)

		// Set OTP in cache with 5-minute expiration
		if err := s.redisService.SetOTP(ctx, otpKey, otp, 5*time.Minute); err != nil {
			return nil, fmt.Errorf("failed to cache OTP: %w", err)
		}
	}

	// Create temporary user for OTP process
	tempUser := entities.NewUser(sendOTPCommand.Username, sendOTPCommand.Email, sendOTPCommand.Password)

	// Send OTP to user
	if err := s.otpService.SendOTP(ctx, sendOTPCommand.Email, otp); err != nil {
		// Clean up the cached OTP if we couldn't send it
		s.redisService.DeleteKey(ctx, otpKey)
		return nil, fmt.Errorf("failed to send OTP: %w", err)
	}

	// Store user data with a longer TTL (15 minutes)
	if err := s.redisService.SetUserData(ctx, sendOTPCommand.Email, tempUser, 15*time.Minute); err != nil {
		return nil, fmt.Errorf("failed to cache user data: %w", err)
	}

	result := command.SendOTPCommandResult{
		Message: "OTP sent successfully",
	}

	// Store response in idempotency record
	if sendOTPCommand.IdempotencyKey != "" {
		requestJSON, _ := json.Marshal(sendOTPCommand)
		idempotencyRecord := entities.NewIdempotencyRecord(sendOTPCommand.IdempotencyKey, string(requestJSON))
		responseJSON, _ := json.Marshal(result)
		idempotencyRecord.SetResponse(string(responseJSON), 200)
		_, err = s.idempotencyRepo.Create(ctx, idempotencyRecord)
		if err != nil {
			log.Printf("Failed to store idempotency record: %v", err)
		}
	}

	return &result, nil
}

func (s *UserService) VerifyOTP(verifyOTPCommand *command.VerifyOTPCommand) (*command.VerifyOTPCommandResult, error) {
	ctx := context.Background()

	// Check idempotency key
	if verifyOTPCommand.IdempotencyKey != "" {
		existingRecord, err := s.idempotencyRepo.FindByKey(ctx, verifyOTPCommand.IdempotencyKey)
		if err != nil {
			return nil, err
		}

		if existingRecord != nil {
			// Return cached response
			var result command.VerifyOTPCommandResult
			if err := json.Unmarshal([]byte(existingRecord.Response), &result); err != nil {
				return nil, err
			}
			return &result, nil
		}
	}

	// Apply rate limiting for OTP verification attempts
	if !s.rateLimiter.Allow("verify:" + verifyOTPCommand.Email) {
		return nil, errors.New("too many verification attempts, please try again later")
	}

	// Get OTP from cache
	otpKey := "otp:" + verifyOTPCommand.Email
	cacheOtp, err := s.redisService.GetOTP(ctx, otpKey)
	if err != nil {
		// If Redis is not available or key doesn't exist, return error
		if err.Error() == "redis: nil" {
			return nil, errors.New("OTP expired or not found")
		}
		return nil, fmt.Errorf("failed to retrieve OTP from cache: %w", err)
	}

	// Check if OTP exists
	if cacheOtp == "" {
		return nil, errors.New("OTP expired or not found")
	}

	// Verify OTP
	isValid, err := s.otpService.VerifyOTP(ctx, verifyOTPCommand.Email, verifyOTPCommand.OTP, cacheOtp)
	if err != nil {
		return nil, fmt.Errorf("OTP verification failed: %w", err)
	}

	if !isValid {
		return nil, errors.New("invalid OTP")
	}

	// If OTP is valid, get user data from cache
	user, err := s.redisService.GetUserData(ctx, verifyOTPCommand.Email)
	if err != nil {
		// If Redis is not available or key doesn't exist, return error
		if err.Error() == "redis: nil" {
			return nil, errors.New("user data expired or not found")
		}
		return nil, fmt.Errorf("failed to retrieve user data: %w", err)
	}

	if user == nil {
		return nil, errors.New("user data expired or not found")
	}

	// Mark user as verified
	user.MarkAsVerified()

	// Create validated user and save to database
	validatedUser, err := entities.NewValidatedUser(user)
	if err != nil {
		return nil, err
	}

	createdUser, err := s.userRepo.Create(validatedUser)
	if err != nil {
		return nil, fmt.Errorf("failed to register user: %w", err)
	}

	// Clean up cache after successful registration
	s.redisService.DeleteKey(ctx, otpKey)
	s.redisService.DeleteKey(ctx, "user:"+verifyOTPCommand.Email)

	result := command.VerifyOTPCommandResult{
		Result: mapper.NewUserResultFromEntity(createdUser),
	}

	// Store response in idempotency record
	if verifyOTPCommand.IdempotencyKey != "" {
		requestJSON, _ := json.Marshal(verifyOTPCommand)
		idempotencyRecord := entities.NewIdempotencyRecord(verifyOTPCommand.IdempotencyKey, string(requestJSON))
		responseJSON, _ := json.Marshal(result)
		idempotencyRecord.SetResponse(string(responseJSON), 200)
		_, err = s.idempotencyRepo.Create(ctx, idempotencyRecord)
		if err != nil {
			log.Printf("Failed to store idempotency record: %v", err)
		}
	}

	return &result, nil
}

func (s *UserService) FindUserById(id uuid.UUID) (*query.UserQueryResult, error) {
	user, err := s.userRepo.FindById(id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	result := query.UserQueryResult{
		Result: mapper.NewUserResultFromEntity(user),
	}

	return &result, nil
}

func (s *UserService) GetProfile(id uuid.UUID) (*query.UserQueryResult, error) {
	ctx := context.Background()

	// First, try to get the profile from Redis cache
	cachedUser, err := s.redisService.GetProfile(ctx, id.String())
	if err == nil && cachedUser != nil {
		// Cache hit, return the cached profile (exclude password)
		cachedUser.Password = ""
		result := query.UserQueryResult{
			Result: mapper.NewUserResultFromEntity(cachedUser),
		}
		return &result, nil
	}
	// If Redis error (like redis: nil), continue to database lookup

	// If not in cache, get it from the database
	user, err := s.userRepo.GetProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	// Cache the user profile in Redis for future access, with TTL
	err = s.redisService.SetProfile(ctx, id.String(), user, 24*time.Hour)
	if err != nil {
		log.Printf("Failed to cache user profile: %v", err)
	}

	result := query.UserQueryResult{
		Result: mapper.NewUserResultFromEntity(user),
	}

	return &result, nil
}
