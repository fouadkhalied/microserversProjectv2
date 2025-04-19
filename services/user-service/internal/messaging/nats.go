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

// PublishCartMessage sends a message to the "cart.created" subject.
func PublishCartMessage(msg string) error {
	// Ensure NATS is connected before publishing
	if nc == nil || !nc.IsConnected() {
		log.Println("❌ NATS connection is closed or not established.")
		return nats.ErrConnectionClosed
	}

	// Publish the message to the "cart.created" subject
	err := nc.Publish("cart.created", []byte(msg))
	if err != nil {
		log.Println("❌ Failed to publish message to cart.created:", err)
		return err
	}

	log.Println("📤 Message successfully published to cart.created:", msg)
	return nil
}

// CloseNats closes the NATS connection gracefully.
func CloseNats() {
	if nc != nil && nc.IsConnected() {
		nc.Close()
		log.Println("✅ NATS connection closed.")
	}
}
