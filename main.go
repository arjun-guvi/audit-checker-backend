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

// workerNamespace is the Redis namespace shared by job producers (HTTP server) and the worker.
const workerNamespace = "audit_worker"

func main() {
	// Load environment variables
	config.LoadEnv()

	workerMode := flag.Bool("worker", false, "Run in worker mode (background job processor)")
	sapWorkerMode := flag.Bool("sap-worker", false, "Run SAP reminder worker (checks every 30 minutes)")
	flag.Parse()

	// Setup Redis pool
	redisPool := &redis.Pool{
    MaxActive: 20,
    MaxIdle:   10,
    Wait:      true,

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

	// Connect to MongoDB
	if err := config.ConnectMongo(); err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer config.DisconnectMongo()

	// Worker mode for Redis-based background jobs
	if *workerMode {
		log.Println("Starting Redis worker...")
		worker.StartWorker(redisPool, workerNamespace)
		return
	}

	// SAP Worker mode for 30-minute reminder checks
	if *sapWorkerMode {
		log.Println("Starting SAP reminder worker...")
		sapWorker := worker.NewSAPWorker()
		if err := sapWorker.Start(); err != nil {
			log.Fatalf("SAP worker error: %v", err)
		}
		return
	}

	// HTTP server mode (default)
	log.Printf("Starting HTTP server on port %s...", config.Port)
	router := gin.Default()
	routes.SetupRoutes(router, redisPool, workerNamespace)
	router.Run(config.Host + ":" + config.Port)
}
