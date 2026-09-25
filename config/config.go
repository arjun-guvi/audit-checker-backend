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
	Host          string
	Port          string

	// Sales Audit
	SalesAuditProgram string
	AccountsEmail     string
	SMTPHost          string
	SMTPPort          string
	SMTPUsername      string
	SMTPPassword      string
	SMTPFrom          string

	// Zoho Creator learner import
	ZohoAPIURL       string
	ZohoAPIPublicKey string

	// OpenAI-compatible chat completions API for the dashboard summaries
	LLMAPIURL string
	LLMAPIKey string
	LLMModel  string

	MongoClient *mongo.Client
	MongoDB     *mongo.Database
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
	// Containers must listen on 0.0.0.0; the default keeps local runs off the network
	Host = getEnv("HOST", "127.0.0.1")
	Port = getEnv("PORT", "8080")

	SalesAuditProgram = getEnv("SALES_AUDIT_PROGRAM", "guvi")
	AccountsEmail = getEnv("ACCOUNTS_EMAIL", "")
	// Leave SMTP_HOST empty to log Sales Audit mails without sending them
	SMTPHost = getEnv("SMTP_HOST", "")
	SMTPPort = getEnv("SMTP_PORT", "587")
	SMTPUsername = getEnv("SMTP_USERNAME", "")
	SMTPPassword = getEnv("SMTP_PASSWORD", "")
	SMTPFrom = getEnv("SMTP_FROM", "")
	ZohoAPIURL = getEnv("ZOHO_API_URL", "https://www.zohoapis.in/creator/custom/teamzen_guvi/Zen_Learner_Data")
	ZohoAPIPublicKey = getEnv("ZOHO_API_PUBLIC_KEY", "")
	// Leave LLM_API_URL or LLM_MODEL empty to turn the dashboard summaries off
	LLMAPIURL = getEnv("LLM_API_URL", "")
	LLMAPIKey = getEnv("LLM_API_KEY", "")
	LLMModel = getEnv("LLM_MODEL", "")

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

	log.Printf("Connected to MongoDB database %q", MongoDatabase)
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
