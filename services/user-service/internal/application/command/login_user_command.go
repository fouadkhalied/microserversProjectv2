package command

import "user-service-new/internal/application/common"

type LoginUserCommand struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginUserCommandResult struct {
	Token string             `json:"token"`
	User  *common.UserResult `json:"user"`
}
