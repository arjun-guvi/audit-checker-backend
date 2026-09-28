package main

import (
	"flag"
	"log"
	"os"

	"auditApp/config"
	"auditApp/routes"
	"auditApp/worker"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gomodule/redigo/redis"
)

// workerNamespace is the Redis namespace shared by job producers (HTTP server) and the worker.
const workerNamespace = "audit_worker"

func main() {
	config.LoadEnv()

	workerMode := flag.Bool("worker", false, "Run only the Redis worker (background jobs)")
	withWorker := flag.Bool("with-worker", false, "Run the HTTP server and the Redis worker in one process")
	flag.Parse()

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

	if err := config.ConnectMongo(); err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer config.DisconnectMongo()

	if *workerMode {
		log.Println("Starting Redis worker...")
		worker.StartWorker(redisPool, workerNamespace)
		return
	}

	// HTTP server with the worker alongside, for hosts running a single service. The worker owns
	// SIGINT/SIGTERM: once it has stopped and its running jobs have finished, the process exits.
	if *withWorker {
		log.Println("Starting Redis worker alongside the HTTP server...")
		go func() {
			worker.StartWorker(redisPool, workerNamespace)
			log.Println("Worker stopped, shutting down")
			config.DisconnectMongo()
			os.Exit(0)
		}()
	}

	log.Printf("Starting HTTP server on port %s...", config.Port)
	router := gin.Default()

	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"https://auditor-client.vercel.app",
			"http://localhost:5173",
		},
		AllowMethods: []string{
			"GET",
			"POST",
			"PUT",
			"PATCH",
			"DELETE",
			"OPTIONS",
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Authorization",
		},
		AllowCredentials: true,
	}))

	routes.SetupRoutes(router, redisPool, workerNamespace)
	if err := router.Run(config.Host + ":" + config.Port); err != nil {
		log.Fatal(err)
	}
}
