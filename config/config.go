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
	JWTSecret     string

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
	ZohoAPIFrom      string
	ZohoAPITo        string

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
	JWTSecret = getEnv("JWT_SECRET", "")

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
	ZohoAPIFrom = getEnv("ZOHO_API_FROM", "20-Sep-2026")
	ZohoAPITo = getEnv("ZOHO_API_TO", "24-Sep-2026")

	// Secrets have no fallback: a missing one must fail loudly, not run with a known value
	if JWTSecret == "" {
		log.Fatal("JWT_SECRET is not set")
	}
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
