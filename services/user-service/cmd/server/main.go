package main

import (
	"log"
	"net/http"
	"user-service/internal/db"
	"user-service/internal/messaging"
)

func main() {
	err := db.Connect()
	if err != nil {
		log.Fatal("❌ Failed to connect to MongoDB:", err)
	}

	err2 := messaging.ConnectNats()

	if err2 != nil {
		log.Fatal("❌ Failed to connect to NATS:", err)
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		err := messaging.PublishCartMessage(`{"message": "New cart created for user 123"}`)
		if err != nil {
			http.Error(w, "❌ Failed to send NATS message", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("User Service with MongoDB is running!"))
	})

	log.Println("🚀 Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
	
}