package main

import (
	"flag"
	"log"

	"researchhub-backend/cmd"
	"researchhub-backend/config"
	"researchhub-backend/routes"
	"researchhub-backend/services"
)

func main() {
	seedOnly := flag.Bool("seed", false, "Run database migrations and development seed data then exit")
	flag.Parse()

	if err := cmd.BootstrapDatabaseAndSeed(); err != nil {
		log.Fatalf("[FATAL] Failed to initialize ResearchHub backend: %v", err)
	}

	if *seedOnly {
		log.Println("[INFO] Seed flag completed. Exiting.")
		return
	}

	services.StartJobWorker()

	r := routes.SetupRouter()
	port := "8080"
	if config.Cfg != nil && config.Cfg.Port != "" {
		port = config.Cfg.Port
	}

	log.Printf("[INFO] ResearchHub Backend API Server listening on http://localhost:%s", port)
	log.Printf("[INFO] Swagger API Documentation available at http://localhost:%s/swagger/index.html", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("[FATAL] Server stopped: %v", err)
	}
}
