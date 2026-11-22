package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/glebarez/sqlite"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type AppConfig struct {
	Port                    string
	AppEnv                  string
	AppName                 string
	AppURL                  string
	APIURL                  string
	DBDriver                string
	DBHost                  string
	DBPort                  string
	DBUser                  string
	DBPassword              string
	DBName                  string
	DBSSLMode               string
	SQLitePath              string
	JWTSecret               string
	JWTExpiryHours          int
	ResetTokenExpiryMinutes int
	StorageDriver           string
	StorageLocalPath        string
	MaxUploadSizeMB         int64
	S3Endpoint              string
	S3Region                string
	S3Bucket                string
	S3AccessKey             string
	S3SecretKey             string
	SMTPEnabled             bool
	SMTPHost                string
	SMTPPort                int
	SMTPUsername            string
	SMTPPassword            string
	SMTPFromEmail           string
	SMTPFromName            string
	CORSAllowedOrigins      string
	SeedDevData             bool
	RazorpayKeyID           string
	RazorpayKeySecret       string
	SubmissionFeeINR        int
}

var (
	Cfg *AppConfig
	DB  *gorm.DB
)

func LoadConfig() *AppConfig {
	_ = godotenv.Load()

	jwtHours, err := strconv.Atoi(getEnv("JWT_EXPIRY_HOURS", "24"))
	if err != nil {
		jwtHours = 24
	}
	resetMin, err := strconv.Atoi(getEnv("RESET_TOKEN_EXPIRY_MINUTES", "60"))
	if err != nil {
		resetMin = 60
	}
	maxMB, err := strconv.ParseInt(getEnv("MAX_UPLOAD_SIZE_MB", "25"), 10, 64)
	if err != nil {
		maxMB = 25
	}
	smtpPort, err := strconv.Atoi(getEnv("SMTP_PORT", "587"))
	if err != nil {
		smtpPort = 587
	}

	Cfg = &AppConfig{
		Port:                    getEnv("PORT", "8080"),
		AppEnv:                  getEnv("APP_ENV", "development"),
		AppName:                 getEnv("APP_NAME", "ResearchHub"),
		AppURL:                  getEnv("APP_URL", "http://localhost:4200"),
		APIURL:                  getEnv("API_URL", "http://localhost:8080/api/v1"),
		DBDriver:                getEnv("DB_DRIVER", "postgres"),
		DBHost:                  getEnv("DB_HOST", "localhost"),
		DBPort:                  getEnv("DB_PORT", "5432"),
		DBUser:                  getEnv("DB_USER", "postgres"),
		DBPassword:              getEnv("DB_PASSWORD", "postgres"),
		DBName:                  getEnv("DB_NAME", "researchhub_db"),
		DBSSLMode:               getEnv("DB_SSLMODE", "disable"),
		SQLitePath:              getEnv("SQLITE_PATH", "./storage/researchhub.db"),
		JWTSecret:               getEnv("JWT_SECRET", "dev_only_researchhub_jwt_secret_key_2026"),
		JWTExpiryHours:          jwtHours,
		ResetTokenExpiryMinutes: resetMin,
		StorageDriver:           getEnv("STORAGE_DRIVER", "local"),
		StorageLocalPath:        getEnv("STORAGE_LOCAL_PATH", "./storage"),
		MaxUploadSizeMB:         maxMB,
		S3Endpoint:              getEnv("S3_ENDPOINT", "https://s3.amazonaws.com"),
		S3Region:                getEnv("S3_REGION", "us-east-1"),
		S3Bucket:                getEnv("S3_BUCKET", "researchhub-files"),
		S3AccessKey:             getEnv("S3_ACCESS_KEY", ""),
		S3SecretKey:             getEnv("S3_SECRET_KEY", ""),
		SMTPEnabled:             getEnv("SMTP_ENABLED", "false") == "true",
		SMTPHost:                getEnv("SMTP_HOST", "smtp.example.com"),
		SMTPPort:                smtpPort,
		SMTPUsername:            getEnv("SMTP_USERNAME", ""),
		SMTPPassword:            getEnv("SMTP_PASSWORD", ""),
		SMTPFromEmail:           getEnv("SMTP_FROM_EMAIL", "noreply@researchhub.org"),
		SMTPFromName:            getEnv("SMTP_FROM_NAME", "ResearchHub Editorial Office"),
		CORSAllowedOrigins:      getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:4200"),
		SeedDevData:             getEnv("SEED_DEV_DATA", "true") == "true",
		RazorpayKeyID:           getEnv("RAZORPAY_KEY_ID", ""),
		RazorpayKeySecret:       getEnv("RAZORPAY_KEY_SECRET", ""),
		SubmissionFeeINR:        1000,
	}

	return Cfg
}

func ConnectDatabase(cfg *AppConfig) (*gorm.DB, error) {
	gormCfg := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	}

	if cfg.DBDriver == "sqlite" {
		db, err := gorm.Open(sqlite.Open(cfg.SQLitePath), gormCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to sqlite database: %w", err)
		}
		DB = db
		return db, nil
	}

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC",
		cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBPort, cfg.DBSSLMode,
	)
	db, err := gorm.Open(postgres.Open(dsn), gormCfg)
	if err != nil {
		// Attempt connecting to default 'postgres' database to auto-create target DB if credentials match
		adminDSN := fmt.Sprintf(
			"host=%s user=%s password=%s dbname=postgres port=%s sslmode=%s TimeZone=UTC",
			cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBPort, cfg.DBSSLMode,
		)
		if adminDB, adminErr := gorm.Open(postgres.Open(adminDSN), gormCfg); adminErr == nil {
			createSQL := fmt.Sprintf("CREATE DATABASE %s", cfg.DBName)
			_ = adminDB.Exec(createSQL).Error
			if sqlDB, sErr := adminDB.DB(); sErr == nil {
				_ = sqlDB.Close()
			}
			db, err = gorm.Open(postgres.Open(dsn), gormCfg)
		}
	}

	if err != nil {
		if cfg.AppEnv == "development" && getEnv("DB_FALLBACK_SQLITE", "true") == "true" {
			log.Printf("[WARN] PostgreSQL connection failed (%v). Falling back to embedded SQLite at %s for development continuity.", err, cfg.SQLitePath)
			_ = os.MkdirAll("./storage", 0755)
			db, sErr := gorm.Open(sqlite.Open(cfg.SQLitePath), gormCfg)
			if sErr != nil {
				return nil, fmt.Errorf("postgres failed (%v) and sqlite fallback failed (%w)", err, sErr)
			}
			DB = db
			return db, nil
		}
		return nil, fmt.Errorf("failed to connect to PostgreSQL database: %w", err)
	}

	log.Printf("[INFO] Connected to PostgreSQL database '%s' at %s:%s", cfg.DBName, cfg.DBHost, cfg.DBPort)
	DB = db
	return db, nil
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}
