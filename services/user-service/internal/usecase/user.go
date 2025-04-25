package usecase

import (
    "context"
    "user-service/internal/domain"
    "user-service/internal/repository"
    "time"
    "golang.org/x/crypto/bcrypt"
    "user-service/internal/infastructure"
)

type UserUsecase struct {
    userRepo  *repository.UserRepo
    redisRepo *repository.RedisRepo
    jwtService *infastructure.JWTService
}

func NewUserUsecase(userRepo *repository.UserRepo, redisRepo *repository.RedisRepo , jwtService *infastructure.JWTService) *UserUsecase {
    return &UserUsecase{userRepo: userRepo, redisRepo: redisRepo , jwtService: jwtService}
}

func (uc *UserUsecase) RegisterUser(ctx context.Context, user *domain.User) error {
    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
    if err != nil {
        return err
    }
    user.Password = string(hashedPassword)
    return uc.userRepo.CreateUser(ctx, user)
}

func (uc *UserUsecase) LoginUser(ctx context.Context, username, password string) (string, error) {
    user, err := uc.userRepo.FindByCredentials(ctx, username)
    if err != nil {
        return "", err
    }

    err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
    if err != nil {
        return "", err
    }

    // Generate JWT token (implementation not shown here)
    token,err := uc.jwtService.GenerateToken(user.ID)
    if err != nil {
        return "",err
    }

    // Store token in Redis with expiration
    err = uc.redisRepo.SetToken(ctx, token, user.ID, time.Hour*24)
    if err != nil {
        return "", err
    }

    // Update tokens in PostgreSQL
    err = uc.userRepo.UpdateTokens(ctx, user.ID, token)
    if err != nil {
        return "", err
    }

    return token, nil
}
