package messaging

import (
	"log"
	"github.com/nats-io/nats.go"
)

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