package config

import (
	"context"
	"log"
	"os"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	RedisHost     string
	RedisPassword string
	MongoURI      string
	MongoDatabase string
	Port          string
	MongoClient   *mongo.Client
	MongoDB       *mongo.Database
)

func LoadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using system env vars")
	}

	RedisHost = getEnv("REDIS_HOST", "localhost:6379")
	RedisPassword = getEnv("REDIS_PASSWORD", "")
	MongoURI = getEnv("MONGO_URI", "mongodb://localhost:27017")
	MongoDatabase = getEnv("MONGO_DATABASE", "audit_app")
	Port = getEnv("PORT", "8080")
}

func ConnectMongo() error {
	clientOptions := options.Client().ApplyURI(MongoURI)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		return err
	}

	err = client.Ping(context.TODO(), nil)
	if err != nil {
		return err
	}

	MongoClient = client
	MongoDB = client.Database(MongoDatabase)

	log.Printf("Connected to MongoDB: %s", MongoURI)
	return nil
}

func DisconnectMongo() {
	if MongoClient != nil {
		MongoClient.Disconnect(context.TODO())
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		value = fallback
	}
	return value
}

// GetEnv is the exported version of getEnv for use by other packages
func GetEnv(key, fallback string) string {
	return getEnv(key, fallback)
}
