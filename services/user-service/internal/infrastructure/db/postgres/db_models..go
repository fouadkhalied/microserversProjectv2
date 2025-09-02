package postgres

import (
	"github.com/google/uuid"
	"time"
)

type IdempotencyRecord struct {
	Id         uuid.UUID `gorm:"primaryKey"`
	Key        string    `gorm:"uniqueIndex"`
	Request    string
	Response   string
	StatusCode int
	CreatedAt  time.Time
}
