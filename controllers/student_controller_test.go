package controllers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"researchhub-backend/config"
	"researchhub-backend/migrations"
	"researchhub-backend/models"
	"researchhub-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupCategoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	tmp := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(tmp, "test.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, sErr := db.DB(); sErr == nil {
			_ = sqlDB.Close()
		}
	})
	config.DB = db
	config.Cfg = &config.AppConfig{
		StorageDriver:    "local",
		StorageLocalPath: filepath.Join(tmp, "storage"),
		MaxUploadSizeMB:  10,
		SeedDevData:      false, // Production mode
	}
	services.InitStorageService(config.Cfg)
	if err := migrations.RunMigrations(db); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}
	return db
}

func TestSeedEssentialCategoriesInProduction(t *testing.T) {
	db := setupCategoryTestDB(t)

	// 1. Fresh production DB seeds 5 essential categories even when SEED_DEV_DATA=false
	if err := migrations.SeedEssentialCategories(db); err != nil {
		t.Fatalf("SeedEssentialCategories failed: %v", err)
	}
	var cats []models.ResearchCategory
	db.Find(&cats)
	if len(cats) != 5 {
		t.Fatalf("expected 5 essential categories, got %d", len(cats))
	}

	// 2. Preserve existing IDs and prevent duplicates on re-run
	firstID := cats[0].ID
	if err := migrations.SeedEssentialCategories(db); err != nil {
		t.Fatalf("second SeedEssentialCategories failed: %v", err)
	}
	var catsAfter []models.ResearchCategory
	db.Find(&catsAfter)
	if len(catsAfter) != 5 {
		t.Fatalf("expected 5 categories after re-seed, got %d", len(catsAfter))
	}
	if catsAfter[0].ID != firstID {
		t.Fatalf("expected existing category ID %s to be preserved, got %s", firstID, catsAfter[0].ID)
	}
}

func submitPaperMultipart(t *testing.T, router *gin.Engine, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	part, _ := w.CreateFormFile("pdf_file", "paper.pdf")
	_, _ = part.Write([]byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF\n"))
	_ = w.Close()

	req, _ := http.NewRequest("POST", "/api/v1/student/papers", &body)
	req.Header.Set("Content-Type", w.Boundary())
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestSubmitPaperCategoryValidationAndCustomCategory(t *testing.T) {
	db := setupCategoryTestDB(t)
	_ = migrations.SeedEssentialCategories(db)

	student := models.User{
		Email:       "student@test.edu",
		FullName:    "Test Student",
		RoleName:    models.RoleStudent,
		Institution: "Test College",
		IsActive:    true,
	}
	if err := db.Create(&student).Error; err != nil {
		t.Fatalf("failed to create student: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/student/papers", func(c *gin.Context) {
		c.Set("user_id", student.ID)
		c.Set("user_name", student.FullName)
		c.Set("user_email", student.Email)
		SubmitResearchPaper(c)
	})

	// 1. Invalid category ID must return HTTP 400 (not 500) and leave no orphaned files
	badRec := submitPaperMultipart(t, r, map[string]string{
		"title":                 "Invalid Category Test",
		"abstract":              "Test abstract",
		"category_id":           "non-existent-id-123",
		"institution":           "Test College",
		"declaration_accepted":  "true",
		"originality_confirmed": "true",
	})
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid category_id, got %d: %s", badRec.Code, badRec.Body.String())
	}
	papersDir := filepath.Join(config.Cfg.StorageLocalPath, "papers")
	entries, _ := os.ReadDir(papersDir)
	if len(entries) != 0 {
		t.Fatalf("expected 0 orphaned PDF files after invalid category rejection, got %d", len(entries))
	}

	// 2. Valid seeded category UUID succeeds (201)
	var aiml models.ResearchCategory
	db.Where("code = ?", "AIML").First(&aiml)
	okRec := submitPaperMultipart(t, r, map[string]string{
		"title":                 "Valid Category Paper",
		"abstract":              "Valid abstract",
		"category_id":           aiml.ID,
		"institution":           "Test College",
		"declaration_accepted":  "true",
		"originality_confirmed": "true",
	})
	if okRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for valid category_id, got %d: %s", okRec.Code, okRec.Body.String())
	}

	// 3. Custom category creates/resolves category and succeeds (201)
	customRec := submitPaperMultipart(t, r, map[string]string{
		"title":                 "Quantum Pharmacy Paper",
		"abstract":              "Custom category abstract",
		"category_id":           "__OTHER__",
		"custom_category_name":  "Quantum Pharmacy",
		"institution":           "Test College",
		"declaration_accepted":  "true",
		"originality_confirmed": "true",
	})
	if customRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for custom category, got %d: %s", customRec.Code, customRec.Body.String())
	}
	var customResp map[string]interface{}
	_ = json.Unmarshal(customRec.Body.Bytes(), &customResp)
	var customCat models.ResearchCategory
	if err := db.Where("name = ?", "Quantum Pharmacy").First(&customCat).Error; err != nil {
		t.Fatalf("expected custom category 'Quantum Pharmacy' to be persisted, got err=%v", err)
	}
}
