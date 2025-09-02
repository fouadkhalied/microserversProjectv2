package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"user-service-new/internal/domain/entities"
	"user-service-new/internal/domain/repositories"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) repositories.UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(user *entities.ValidatedUser) (*entities.User, error) {
	userEntity := user.GetUser()

	// Hash password before saving
	if err := userEntity.HashPassword(); err != nil {
		return nil, err
	}

	userModel := UserModel{
		Id:         userEntity.Id,
		CreatedAt:  userEntity.CreatedAt,
		UpdatedAt:  userEntity.UpdatedAt,
		Username:   userEntity.Username,
		Email:      userEntity.Email,
		Password:   userEntity.Password,
		Tokens:     userEntity.Tokens,
		IsVerified: userEntity.IsVerified,
	}

	if err := r.db.Create(&userModel).Error; err != nil {
		return nil, err
	}

	// Read back the created user to ensure data integrity
	return r.FindById(userEntity.Id)
}

func (r *UserRepository) FindById(id uuid.UUID) (*entities.User, error) {
	var userModel UserModel
	if err := r.db.Where("id = ?", id).First(&userModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return r.mapToEntity(&userModel), nil
}

func (r *UserRepository) FindByUsername(username string) (*entities.User, error) {
	var userModel UserModel
	if err := r.db.Where("username = ?", username).First(&userModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return r.mapToEntity(&userModel), nil
}

func (r *UserRepository) FindByEmail(email string) (*entities.User, error) {
	var userModel UserModel
	if err := r.db.Where("email = ?", email).First(&userModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return r.mapToEntity(&userModel), nil
}

func (r *UserRepository) FindByCredentials(username string) (*entities.User, error) {
	return r.FindByUsername(username)
}

func (r *UserRepository) Update(user *entities.ValidatedUser) (*entities.User, error) {
	userEntity := user.GetUser()

	userModel := UserModel{
		Id:         userEntity.Id,
		CreatedAt:  userEntity.CreatedAt,
		UpdatedAt:  userEntity.UpdatedAt,
		Username:   userEntity.Username,
		Email:      userEntity.Email,
		Password:   userEntity.Password,
		Tokens:     userEntity.Tokens,
		IsVerified: userEntity.IsVerified,
	}

	if err := r.db.Save(&userModel).Error; err != nil {
		return nil, err
	}

	// Read back the updated user to ensure data integrity
	return r.FindById(userEntity.Id)
}

func (r *UserRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&UserModel{}, "id = ?", id).Error
}

func (r *UserRepository) UpdateTokens(ctx context.Context, userID uuid.UUID, token string) error {
	return r.db.Model(&UserModel{}).Where("id = ?", userID).Update("tokens", gorm.Expr("array_append(tokens, ?)", token)).Error
}

func (r *UserRepository) GetProfile(ctx context.Context, userID uuid.UUID) (*entities.User, error) {
	return r.FindById(userID)
}

func (r *UserRepository) mapToEntity(userModel *UserModel) *entities.User {
	return &entities.User{
		Id:         userModel.Id,
		CreatedAt:  userModel.CreatedAt,
		UpdatedAt:  userModel.UpdatedAt,
		Username:   userModel.Username,
		Email:      userModel.Email,
		Password:   userModel.Password,
		Tokens:     userModel.Tokens,
		IsVerified: userModel.IsVerified,
	}
}
