package infrastructure

import (
    "log"
    "time"
    "os"
    "github.com/golang-jwt/jwt/v5"
)

type JWTService struct {
    secretKey string
}

func NewJWTService() *JWTService {
    return &JWTService{
        secretKey: os.Getenv("JWTSECRETKEY"),
    }
}

func (j *JWTService) GenerateToken(userID string) (string, error) {
    claims := jwt.MapClaims{
        "user_id": userID,
        "exp":     time.Now().Add(time.Hour * 24).Unix(),
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    log.Println(j.secretKey)
    return token.SignedString([]byte("fouad"))
}
