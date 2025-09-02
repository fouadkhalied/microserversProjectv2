package command

type SendOTPCommand struct {
	Username       string `json:"username"`
	Email          string `json:"email"`
	Password       string `json:"password"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type SendOTPCommandResult struct {
	Message string `json:"message"`
}
