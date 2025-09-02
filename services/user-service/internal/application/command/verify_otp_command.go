package command

import "user-service-new/internal/application/common"

type VerifyOTPCommand struct {
	Email          string `json:"email"`
	OTP            string `json:"otp"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type VerifyOTPCommandResult struct {
	Result *common.UserResult `json:"result"`
}
