
package db

import (
	"context"
	"fmt"
	"time"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var Client *mongo.Client

func Connect() error {
	// Update the URI if you're using Docker
	uri := "mongodb+srv://foukha49:z0ZfOoRSY2hDvtPl@clusterchatapp.oj1e1.mongodb.net/"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return err
	}
	// Check connection
	if err := client.Ping(ctx, nil); err != nil {
		return err
	}

	Client = client
	fmt.Println("✅ Connected to MongoDB!")
	return nil
}

func GetCollection(database string, collection string) *mongo.Collection {
	return Client.Database(database).Collection(collection)
}




