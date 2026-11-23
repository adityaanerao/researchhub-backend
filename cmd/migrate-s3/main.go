package main

import (
	"fmt"
	"log"

	"researchhub-backend/cmd"
	"researchhub-backend/config"
	"researchhub-backend/services"
)

func main() {
	cfg := config.LoadConfig()
	if _, err := config.ConnectDatabase(cfg); err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	services.InitStorageService(cfg)

	report := cmd.MigrateLocalFilesToS3()
	fmt.Printf("Migration complete: scanned=%d uploaded=%d skipped=%d failed=%d\n",
		report.TotalScanned, report.Uploaded, report.Skipped, report.Failed)
	for _, e := range report.Errors {
		fmt.Printf("ERROR: %s\n", e)
	}
}
