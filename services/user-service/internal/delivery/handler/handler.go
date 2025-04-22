package handler

import (
    "context"
    "encoding/json"
    "net/http"
    "user-service/internal/domain"
    "user-service/internal/usecase"
)

type Handler struct {
    uc *usecase.UserUsecase
}

func NewHandler(uc *usecase.UserUsecase) *Handler {
    return &Handler{uc}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
    var user domain.User
    json.NewDecoder(r.Body).Decode(&user)

    err := h.uc.RegisterUser(context.Background(), &user)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(map[string]string{"message": "User created"})
}


func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
    var user domain.User
    json.NewDecoder(r.Body).Decode(&user)

    token,err := h.uc.LoginUser(context.Background(), &user)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(map[string]string{"message": "User logged in" , "token" : token})
}
