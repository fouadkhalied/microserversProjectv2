package mapper

import (
	"user-service-new/internal/application/common"
	"user-service-new/internal/domain/entities"
)

func NewUserResultFromEntity(user *entities.User) *common.UserResult {
	return &common.UserResult{
		Id:         user.Id,
		CreatedAt:  user.CreatedAt,
		UpdatedAt:  user.UpdatedAt,
		Username:   user.Username,
		Email:      user.Email,
		IsVerified: user.IsVerified,
	}
}

func NewUserResultFromValidatedEntity(validatedUser *entities.ValidatedUser) *common.UserResult {
	return NewUserResultFromEntity(validatedUser.GetUser())
}
