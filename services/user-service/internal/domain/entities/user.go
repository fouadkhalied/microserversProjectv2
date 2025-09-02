package entities

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Id         uuid.UUID
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Username   string
	Email      string
	Password   string
	Tokens     []string
	IsVerified bool
}

func NewUser(username, email, password string) *User {
	return &User{
		Id:         uuid.New(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Username:   username,
		Email:      email,
		Password:   password,
		Tokens:     make([]string, 0),
		IsVerified: false,
	}
}

func (u *User) validate() error {
	if u.Username == "" {
		return errors.New("username must not be empty")
	}
	if u.Email == "" {
		return errors.New("email must not be empty")
	}
	if u.Password == "" {
		return errors.New("password must not be empty")
	}
	if u.CreatedAt.After(u.UpdatedAt) {
		return errors.New("created_at must be before updated_at")
	}
	return nil
}

func (u *User) HashPassword() error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hashedPassword)
	return nil
}

func (u *User) CheckPassword(password string) error {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
}

func (u *User) AddToken(token string) {
	u.Tokens = append(u.Tokens, token)
	u.UpdatedAt = time.Now()
}

func (u *User) MarkAsVerified() {
	u.IsVerified = true
	u.UpdatedAt = time.Now()
}

func (u *User) UpdateProfile(username, email string) error {
	u.Username = username
	u.Email = email
	u.UpdatedAt = time.Now()
	return u.validate()
}
