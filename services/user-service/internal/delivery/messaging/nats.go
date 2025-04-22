package messaging

import (
	"log"
	"github.com/nats-io/nats.go"
	"context"
    "encoding/json"
    "user-service/internal/domain"
    "user-service/internal/usecase"
)

var nc *nats.Conn

type Handler struct {
    userUC usecase.UserUsecase
}

// ConnectNats establishes a NATS connection and keeps it open for future use.
func ConnectNats() (*nats.Conn,error) {
	var err error
	// Check if the connection is already established
	if nc != nil && nc.IsConnected() {
		log.Println("✅ NATS already connected.")
		return nc , err
	}

	// Establish the NATS connection
	nc, err = nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Println("❌ Failed to connect to NATS:", err)
		return nil , err
	}

	log.Println("✅ Connected to NATS.")
	return nc,err
}

// Recive message from nats for register

func NewHandler(nc *nats.Conn, userUC usecase.UserUsecase) {
    h := &Handler{userUC: userUC}

    // Subscribe to "user.register"
    _,err := nc.Subscribe("user.register", h.handleRegister)

    if err != nil {
        log.Println("❌ Failed to subscribe to user.register",err)
    }

    // Subscribe to "user.login"
    nc.Subscribe("user.login", h.handleLogin)
}

func (h *Handler) handleRegister(msg *nats.Msg) {
    var user domain.User
    if err := json.Unmarshal(msg.Data, &user); err != nil {
        log.Println("Error decoding user:", err)
        msg.Respond([]byte(`{"error":"invalid input"}`))
        return
    }

    err := h.userUC.RegisterUser(context.TODO(), &user)
    if err != nil {
        log.Println("Register failed:", err)
        msg.Respond([]byte(`{"error":"registration failed"}`))
        return
    }

    msg.Respond([]byte(`{"status":"registered"}`))
}

// login logic 
func (h *Handler) handleLogin(msg *nats.Msg)  {
    var user domain.User

    if err := json.Unmarshal(msg.Data, &user); err != nil {
        log.Println("Error decoding user:", err)
        msg.Respond([]byte(`{"error":"invalid input"}`))
        return
    }

    token,err := h.userUC.LoginUser(context.TODO(),&user)

    if err != nil {
        log.Println("Login failed:", err)
        msg.Respond([]byte(`{"error":"login failed"}`))
        return
    }

    response, _ := json.Marshal(map[string]string{"token": token})
    msg.Respond(response)
}

// CloseNats closes the NATS connection gracefully.
func CloseNats() {
	if nc != nil && nc.IsConnected() {
		nc.Close()
		log.Println("✅ NATS connection closed.")
	}
}
