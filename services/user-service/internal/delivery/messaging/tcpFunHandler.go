package tcp

import (
	"encoding/json"
	"user-service/internal/domain"
	"fmt"
	"context"
)

// handleRegister processes registration requests
func (h *TCPHandler) handleRegister(ctx context.Context, content []byte) (interface{}, error) {
	// Get user object from pool
	user := h.msgPool.Get().(*domain.User)
	defer h.msgPool.Put(user)
	*user = domain.User{} // Reset fields

	if err := json.Unmarshal(content, user); err != nil {
		return nil, fmt.Errorf("invalid input data: %v", err)
	}

	// Validate user data
	if user.Username == "" || user.Password == "" {
		return nil, fmt.Errorf("username and password are required")
	}

	// if err := h.userUC.RegisterUser(ctx, user); err != nil {
	// 	return nil, fmt.Errorf("registration failed: %v", err)
	// }

	if err := h.userUC.SendOTPtoUser(ctx, user); err != nil {
		return nil, fmt.Errorf("registration failed: %v", err)
	}


	// Use a struct instead of map for better performance
	return struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}{
		Status:  "success",
		Message: "User registered successfully",
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

	token, err := h.userUC.LoginUser(ctx, credentials.Username, credentials.Password)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %v", err)
	}

	// Use a struct instead of map for better performance
	return struct {
		Status string `json:"status"`
		Token  string `json:"token"`
	}{
		Status: "success",
		Token:  token,
	}, nil
}

func (h *TCPHandler) handleProfile(ctx context.Context, content []byte) (interface{},error)  {
	var credentials struct {
		UserID string `json:"userID"`
	}

	if err := json.Unmarshal(content,&credentials); err != nil {
		return nil, fmt.Errorf("invalid input data: %v", err)
	}

	user , err := h.userUC.GetProfile(ctx,credentials.UserID); 
	
	if err != nil {
		return nil, fmt.Errorf("error in getting profile : %v", err)
	}

	return struct {
		Status string `json:"status"`
		User  *domain.User `json:"user"`
	}{
		Status: "success",
		User:  user,
	}, nil

}

func (h *TCPHandler) handleEmailOTP(ctx context.Context, content []byte) (interface{},error) {
	var credentials struct {
		Email string `json:"email"`
		OTP string `json:"otp"`
	}

	if err := json.Unmarshal(content,&credentials); err != nil {
		return nil, fmt.Errorf("invalid input data: %v", err)
	}

	err := h.userUC.VerifyOtp(ctx,credentials.Email,credentials.OTP); 
	
	if err != nil {
		return nil, fmt.Errorf("error in verifying otp : %v", err)
	}

	return struct {
		Status string `json:"status"`
		User  bool `json:"user"`
	}{
		Status: "success",
		User:  true,
	}, nil
}