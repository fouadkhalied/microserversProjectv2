package repository

import (
	"context"
	"time"
    "log"
	"user-service/internal/domain"
    "os"
    "fmt"
	jwt "github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
    "go.mongodb.org/mongo-driver/bson/primitive"
)

type UserRepo struct {
    collection *mongo.Collection
}

func NewUserRepo(db *mongo.Database) *UserRepo {
    return &UserRepo{
        collection: db.Collection("users"),
    }
}

func (r *UserRepo) CreateUser(ctx context.Context, user *domain.User) error {
    _, err := r.collection.InsertOne(ctx, user)
    return err
}

func (r *UserRepo) FindByCredintials(ctx context.Context , user *domain.User) (*domain.User, error) {
    var foundUser domain.User

    // Query MongoDB to find the user by username
    err := r.collection.FindOne(ctx, bson.M{"username": user.Username}).Decode(&foundUser)
    if err != nil {
        // Return a specific error if user is not found
        return nil, fmt.Errorf("user not found: %v", err)
    }

    // Compare the stored password hash with the provided password
    err = bcrypt.CompareHashAndPassword([]byte(foundUser.Password), []byte(user.Password))
    if err != nil {
        // Return a general authentication error, do not specify password mismatch
        return nil, fmt.Errorf("authentication failed: %v", err)
    }

    // Return the found user if authentication is successful
    return &foundUser, nil
}


func (r *UserRepo) GenerateToken(ctx context.Context , user *domain.User) (string,error)  {
    claims := jwt.MapClaims{
        "userID" : user.ID,
        "exp" : time.Now().Add(time.Hour * 72).Unix(),
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256 , claims)

    signedToken,err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))

    if err != nil {
        return "",err
    }

    if user.Tokens == nil {
        user.Tokens = []string{}
    }
    

    user.Tokens = append(user.Tokens, signedToken)

     // Save updated token list to MongoDB
    
     log.Println(user.ID)
     id, err := primitive.ObjectIDFromHex(user.ID)
     if err != nil {
         return "", err
     }
     
     _, err = r.collection.UpdateByID(ctx, id, bson.M{
         "$push": bson.M{"tokens": signedToken}, // or "$set" if you're replacing the array
     })

    return signedToken,err
}
