package main

import (
    "log"
    "net/http"
    "user-service/internal/config"
    "user-service/internal/db"
    "user-service/internal/delivery/handler"
    "user-service/internal/delivery/messaging"
    "user-service/internal/repository"
    "user-service/internal/usecase"
    "github.com/gorilla/mux"
)

func main() {
    cfg := config.Load()
    client := db.Connect(cfg.MongoURI)
    database := client.Database("userservice")
    natsConn , err := messaging.ConnectNats()

    if err != nil {
        log.Println("❌ Failed to connect to NATS")
    }

    userRepo := repository.NewUserRepo(database)
    userUC := usecase.NewUserUsecase(userRepo)
    handler := handler.NewHandler(userUC)

    // Register NATS subscriptions
    messaging.NewHandler(natsConn, *userUC)

    // REST API
    r := mux.NewRouter()
    r.HandleFunc("/user/register", handler.Register).Methods("POST")
    r.HandleFunc("/user/login", handler.Login).Methods("POST")

    go func() {
        log.Println("User Service listening on port 3001")
        log.Println(http.ListenAndServe(":3001", r))
    }()

    // Keep the service running
    select {}
}
