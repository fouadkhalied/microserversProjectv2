package postgres

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserModel struct {
	Id         uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
	Username   string         `gorm:"uniqueIndex;not null"`
	Email      string         `gorm:"uniqueIndex;not null"`
	Password   string         `gorm:"not null"`
	Tokens     []string       `gorm:"type:text[]"`
	IsVerified bool           `gorm:"default:false"`
}

func (UserModel) TableName() string {
	return "users"
}
