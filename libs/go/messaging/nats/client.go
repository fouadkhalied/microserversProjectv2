package messaging

import (
	"log"
	"github.com/nats-io/nats.go"
)

var nc *nats.Conn

// ConnectNats establishes a NATS connection and keeps it open for future use.
func ConnectNats() error {
	var err error
	// Check if the connection is already established
	if nc != nil && nc.IsConnected() {
		log.Println("✅ NATS already connected.")
		return nil
	}

	// Establish the NATS connection
	nc, err = nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatal("❌ Failed to connect to NATS:", err)
		return err
	}

	log.Println("✅ Connected to NATS.")
	return nil
}

// CloseNats closes the NATS connection gracefully.
func CloseNats() {
	if nc != nil && nc.IsConnected() {
		nc.Close()
		log.Println("✅ NATS connection closed.")
	}
}

// GetConnection returns the current NATS connection
func GetConnection() *nats.Conn {
	return nc
}