package main

import (
	"flag"
	"log"

	"auditApp/config"
	"auditApp/routes"
	"auditApp/worker"

	"github.com/gin-gonic/gin"
	"github.com/gomodule/redigo/redis"
)

func main() {
	// Load environment variables
	config.LoadEnv()

	workerMode := flag.Bool("worker", false, "Run in worker mode (background job processor)")
	flag.Parse()

	// Setup Redis pool
	redisPool := &redis.Pool{
		MaxActive: 5,
		MaxIdle:   5,
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", config.RedisHost)
			if err != nil {
				return nil, err
			}
			if config.RedisPassword != "" {
				if _, err := conn.Do("AUTH", config.RedisPassword); err != nil {
					conn.Close()
					return nil, err
				}
			}
			return conn, nil
		},
	}

	if *workerMode {
		log.Println("Starting worker...")
		worker.StartWorker(redisPool, "audit_worker")
		return
	}

	// Connect to MongoDB
	// if err := config.ConnectMongo(); err != nil {
	// 	log.Fatalf("Failed to connect to MongoDB: %v", err)
	// }
	// defer config.DisconnectMongo()

	// HTTP server mode (default)
	log.Printf("Starting HTTP server on port %s...", config.Port)
	router := gin.Default()
	routes.SetupRoutes(router)
	router.Run("127.0.0.1:" + config.Port)
}
