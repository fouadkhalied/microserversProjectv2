package interfaces

import (
	"github.com/google/uuid"
	"user-service-new/internal/application/command"
	"user-service-new/internal/application/query"
)

type UserService interface {
	CreateUser(createCommand *command.CreateUserCommand) (*command.CreateUserCommandResult, error)
	LoginUser(loginCommand *command.LoginUserCommand) (*command.LoginUserCommandResult, error)
	SendOTP(sendOTPCommand *command.SendOTPCommand) (*command.SendOTPCommandResult, error)
	VerifyOTP(verifyOTPCommand *command.VerifyOTPCommand) (*command.VerifyOTPCommandResult, error)
	FindUserById(id uuid.UUID) (*query.UserQueryResult, error)
	GetProfile(id uuid.UUID) (*query.UserQueryResult, error)
}
