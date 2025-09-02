package tcp

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"user-service-new/internal/application/command"
)

// handleRegister processes registration requests
func (h *TCPHandler) handleRegister(ctx context.Context, content []byte) (interface{}, error) {
	var userData struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.Unmarshal(content, &userData); err != nil {
		return nil, fmt.Errorf("invalid input data: %v", err)
	}

	// Validate user data
	if userData.Username == "" || userData.Password == "" || userData.Email == "" {
		return nil, fmt.Errorf("username, email and password are required")
	}

	// Create command for sending OTP
	sendOTPCommand := &command.SendOTPCommand{
		Username: userData.Username,
		Email:    userData.Email,
		Password: userData.Password,
	}

	// Send OTP to user
	result, err := h.userService.SendOTP(sendOTPCommand)
	if err != nil {
		return nil, fmt.Errorf("registration failed: %v", err)
	}

	return struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}{
		Status:  "success",
		Message: result.Message,
	}, nil
}

// handleLogin processes login requests
func (h *TCPHandler) handleLogin(ctx context.Context, content []byte) (interface{}, error) {
	var credentials struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.Unmarshal(content, &credentials); err != nil {
		return nil, fmt.Errorf("invalid input data: %v", err)
	}

	if credentials.Username == "" || credentials.Password == "" {
		return nil, fmt.Errorf("missing username or password")
	}

	// Create login command
	loginCommand := &command.LoginUserCommand{
		Username: credentials.Username,
		Password: credentials.Password,
	}

	result, err := h.userService.LoginUser(loginCommand)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %v", err)
	}

	return struct {
		Status string `json:"status"`
		Token  string `json:"token"`
		User   interface{} `json:"user"`
	}{
		Status: "success",
		Token:  result.Token,
		User:   result.User,
	}, nil
}

// handleProfile processes profile requests
func (h *TCPHandler) handleProfile(ctx context.Context, content []byte) (interface{}, error) {
	var request struct {
		UserID string `json:"userID"`
	}

	if err := json.Unmarshal(content, &request); err != nil {
		return nil, fmt.Errorf("invalid input data: %v", err)
	}

	if request.UserID == "" {
		return nil, fmt.Errorf("userID is required")
	}

	// Parse UUID
	userID, err := uuid.Parse(request.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid userID format: %v", err)
	}

	result, err := h.userService.GetProfile(userID)
	if err != nil {
		return nil, fmt.Errorf("error in getting profile: %v", err)
	}

	return struct {
		Status string `json:"status"`
		User   interface{} `json:"user"`
	}{
		Status: "success",
		User:   result.Result,
	}, nil
}

// handleEmailOTP processes OTP verification requests
func (h *TCPHandler) handleEmailOTP(ctx context.Context, content []byte) (interface{}, error) {
	var credentials struct {
		Email string `json:"email"`
		OTP   string `json:"otp"`
	}

	if err := json.Unmarshal(content, &credentials); err != nil {
		return nil, fmt.Errorf("invalid input data: %v", err)
	}

	if credentials.Email == "" || credentials.OTP == "" {
		return nil, fmt.Errorf("email and OTP are required")
	}

	// Create verify OTP command
	verifyOTPCommand := &command.VerifyOTPCommand{
		Email: credentials.Email,
		OTP:   credentials.OTP,
	}

	result, err := h.userService.VerifyOTP(verifyOTPCommand)
	if err != nil {
		return nil, fmt.Errorf("error in verifying OTP: %v", err)
	}

	return struct {
		Status string `json:"status"`
		User   interface{} `json:"user"`
	}{
		Status: "success",
		User:   result.Result,
	}, nil
}
