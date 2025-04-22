package usecase

import (
	"context"
	"log"
	"user-service/internal/domain"
	"user-service/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

type UserUsecase struct {
    repo *repository.UserRepo
}

func NewUserUsecase(repo *repository.UserRepo) *UserUsecase {
    return &UserUsecase{repo}
}

func (uc *UserUsecase) RegisterUser(ctx context.Context, user *domain.User) error {
    // Password hash 
    hashedPassword , err := bcrypt.GenerateFromPassword([]byte(user.Password),bcrypt.DefaultCost) 
    if err != nil {
        log.Println("failed to hash password")
        return err
    }

    user.Password = string(hashedPassword)

    if user.Tokens == nil {
        user.Tokens = []string{} // Initialize as empty array
    }
    return uc.repo.CreateUser(ctx, user)
}

func (uc * UserUsecase) LoginUser(ctx context.Context , user * domain.User) (string , error) {

    // find user and check password
    myUser,err := uc.repo.FindByCredintials(ctx,user)

    if err != nil {
        log.Println("❌ Cannot find user",err)
        return "",err
    }

    // generate token 
    token,err := uc.repo.GenerateToken(ctx,myUser)

    if err != nil {
        log.Println("❌ Cannot generate token")
    }

    return token,err
}
