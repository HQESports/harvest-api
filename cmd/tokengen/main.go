package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const MONGO_URI = "mongodb://username:password@localhost:27017"

func main() {
	// Generate random token
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		log.Fatalf("Error generating token: %v", err)
	}
	token := base64.URLEncoding.EncodeToString(b)

	// Hash the token
	h := sha256.New()
	h.Write([]byte(token))
	tokenHash := hex.EncodeToString(h.Sum(nil))

	// Connect to MongoDB
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(MONGO_URI))
	if err != nil {
		log.Fatalf("Error connecting to MongoDB: %v", err)
	}
	defer client.Disconnect(context.Background())

	// Insert token document
	collection := client.Database("harvest").Collection("api_tokens")
	_, err = collection.InsertOne(context.Background(), bson.M{
		"_id":        primitive.NewObjectID(),
		"token_hash": tokenHash,
		"name":       "Initial Admin Token",
		"role":       "admin",
		"created_at": time.Now(),
		"revoked":    false,
	})
	if err != nil {
		log.Fatalf("Error inserting token: %v", err)
	}

	fmt.Println("Admin token created successfully!")
	fmt.Println("Token:", token)
	fmt.Println("IMPORTANT: Save this token securely. It won't be shown again.")
}
