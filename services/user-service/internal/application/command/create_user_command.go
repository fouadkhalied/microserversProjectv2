package command

import "user-service-new/internal/application/common"

type CreateUserCommand struct {
	Username       string `json:"username"`
	Email          string `json:"email"`
	Password       string `json:"password"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type CreateUserCommandResult struct {
	Result *common.UserResult `json:"result"`
}
