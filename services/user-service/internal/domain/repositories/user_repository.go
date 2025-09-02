package repositories

import (
	"context"

	"github.com/google/uuid"
	"user-service-new/internal/domain/entities"
)

type UserRepository interface {
	Create(user *entities.ValidatedUser) (*entities.User, error)
	FindById(id uuid.UUID) (*entities.User, error)
	FindByUsername(username string) (*entities.User, error)
	FindByEmail(email string) (*entities.User, error)
	FindByCredentials(username string) (*entities.User, error)
	Update(user *entities.ValidatedUser) (*entities.User, error)
	Delete(id uuid.UUID) error
	UpdateTokens(ctx context.Context, userID uuid.UUID, token string) error
	GetProfile(ctx context.Context, userID uuid.UUID) (*entities.User, error)
}
