package cmd

import (
	"log"

	"researchhub-backend/config"
	"researchhub-backend/migrations"
	"researchhub-backend/services"
)

func BootstrapDatabaseAndSeed() error {
	cfg := config.LoadConfig()
	services.InitStorageService(cfg)

	db, err := config.ConnectDatabase(cfg)
	if err != nil {
		return err
	}

	if err := migrations.RunMigrations(db); err != nil {
		return err
	}

	if err := migrations.SeedEssentialCategories(db); err != nil {
		return err
	}

	if err := migrations.SeedDevelopmentData(db); err != nil {
		return err
	}

	log.Println("[INFO] ResearchHub database bootstrap and seeding complete.")
	return nil
}
