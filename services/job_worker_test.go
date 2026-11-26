package services

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"researchhub-backend/config"
	"researchhub-backend/models"
	"researchhub-backend/utils"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestExecuteFormattingJobPreservesS3LogoWhenOptionsJSONHasEmptyLogoPath(t *testing.T) {
	tmp := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(tmp, "job_test.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, sErr := db.DB(); sErr == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.SystemSetting{}, &models.ResearchPaper{}, &models.PaperFile{}, &models.Publication{}, &models.FormattingJob{}); err != nil {
		t.Fatalf("automigrate failed: %v", err)
	}
	config.DB = db
	config.Cfg = &config.AppConfig{
		AppURL:           "https://researchhub.buzz",
		StorageDriver:    "s3",
		StorageLocalPath: tmp,
		S3Bucket:         "researchimage",
		S3Region:         "ap-southeast-2",
	}

	mockS3 := newMockS3Client()
	Storage = &S3StorageService{
		Client:   mockS3,
		Bucket:   "researchimage",
		Region:   "ap-southeast-2",
		BaseDir:  tmp,
		MaxBytes: 10 * 1024 * 1024,
	}

	// 1. Seed official S3 logo at branding/logo.png and SystemSetting publication_logo=branding/logo.png
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	img.Set(0, 0, color.RGBA{R: 11, G: 31, B: 58, A: 255})
	var logoBuf bytes.Buffer
	_ = png.Encode(&logoBuf, img)
	logoCT := "image/png"
	logoKey := "branding/logo.png"
	_, _ = mockS3.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &config.Cfg.S3Bucket,
		Key:         &logoKey,
		Body:        bytes.NewReader(logoBuf.Bytes()),
		ContentType: &logoCT,
	})
	_ = db.Create(&models.SystemSetting{SettingKey: "publication_logo", SettingValue: "branding/logo.png"}).Error

	// 2. Upload original student PDF to mocked S3
	samplePDFPath, _, err := utils.GenerateSamplePaperPDF(tmp, "RH-2026-S3", "S3 Logo Regression Paper", "Test Author", "Test Institute", "Abstract", "S3")
	if err != nil {
		t.Fatalf("failed to generate sample PDF: %v", err)
	}
	origBytes, _ := os.ReadFile(samplePDFPath)
	pdfCT := "application/pdf"
	origKey := "papers/paper-1/original/orig.pdf"
	_, _ = mockS3.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &config.Cfg.S3Bucket,
		Key:         &origKey,
		Body:        bytes.NewReader(origBytes),
		ContentType: &pdfCT,
	})

	paper := models.ResearchPaper{SubmissionCode: "RH-2026-S3", Title: "S3 Logo Regression Paper", Institution: "Test Institute", StudentID: "stu-1", CategoryID: "cat-1"}
	_ = db.Create(&paper).Error
	pf := models.PaperFile{PaperID: paper.ID, StoragePath: origKey, OriginalName: "orig.pdf"}
	_ = db.Create(&pf).Error
	pub := models.Publication{
		PublicationCode:  "RHPUB-2026-S3",
		PaperID:          paper.ID,
		PublicationTitle: paper.Title,
		JournalName:      "International Journal of Academic Research & Innovation",
		Volume:           "12",
		Issue:            "4",
		PublicationYear:  2026,
		PublicationDate:  time.Now(),
	}
	_ = db.Create(&pub).Error

	// 3. Create FormattingJob with empty logo_path in OptionsJSON (regression case)
	job := models.FormattingJob{
		PaperID:       paper.ID,
		PublicationID: pub.ID,
		SourceFileID:  pf.ID,
		Status:        models.FormattingQueued,
		OptionsJSON:   `{"header_enabled":true,"footer_enabled":true,"qr_enabled":true,"page_numbers_enabled":true,"watermark_enabled":true,"logo_path":""}`,
		MaxAttempts:   3,
	}
	_ = db.Create(&job).Error

	if err := ExecuteFormattingJob(&job); err != nil {
		t.Fatalf("ExecuteFormattingJob failed: %v", err)
	}

	var updatedPub models.Publication
	_ = db.Where("id = ?", pub.ID).First(&updatedPub).Error
	if updatedPub.FormattingStatus != models.FormattingGenerated {
		t.Fatalf("expected GENERATED status, got %s (err=%s)", updatedPub.FormattingStatus, updatedPub.FormattingError)
	}
	if _, ok := mockS3.objects[updatedPub.FormattedFilePath]; !ok {
		t.Fatalf("expected branded PDF to be uploaded to S3 at %s", updatedPub.FormattedFilePath)
	}
	if _, ok := mockS3.objects[origKey]; !ok {
		t.Fatalf("expected original PDF at %s to be preserved", origKey)
	}
}
