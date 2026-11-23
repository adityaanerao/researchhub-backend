package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"researchhub-backend/config"
	"researchhub-backend/models"
	"researchhub-backend/services"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// MigrationReport summarizes the results of migrating local files to S3.
type MigrationReport struct {
	TotalScanned int
	Uploaded     int
	Skipped      int
	Failed       int
	Errors       []string
}

// MigrateLocalFilesToS3 safely uploads existing local files referenced in the DB to S3
// without overwriting existing S3 objects or deleting original local files.
func MigrateLocalFilesToS3() MigrationReport {
	report := MigrationReport{}
	s3Svc, ok := services.Storage.(*services.S3StorageService)
	if !ok || s3Svc == nil {
		report.Errors = append(report.Errors, "STORAGE_DRIVER must be set to s3 to run migration")
		return report
	}

	migrateOne := func(localPath, subFolder, contentType string) (string, bool) {
		report.TotalScanned++
		if strings.TrimSpace(localPath) == "" {
			report.Skipped++
			return "", false
		}
		// Already an S3-style key without local file on disk
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			report.Skipped++
			return "", false
		}

		fileName := filepath.Base(localPath)
		targetKey := path.Join(subFolder, fileName)

		// Avoid overwriting an existing S3 object
		if _, headErr := s3Svc.Client.HeadObject(context.Background(), &s3.HeadObjectInput{
			Bucket: &s3Svc.Bucket,
			Key:    &targetKey,
		}); headErr == nil {
			report.Skipped++
			return targetKey, true
		}

		_, objKey, err := s3Svc.SaveGeneratedFile(localPath, subFolder, contentType)
		if err != nil {
			report.Failed++
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", localPath, err))
			return "", false
		}
		report.Uploaded++
		return objKey, true
	}

	// 1. PaperFiles
	var paperFiles []models.PaperFile
	if err := config.DB.Find(&paperFiles).Error; err == nil {
		for _, pf := range paperFiles {
			if key, updated := migrateOne(pf.StoragePath, "papers", "application/pdf"); updated {
				_ = config.DB.Model(&pf).Update("storage_path", key).Error
			}
		}
	}

	// 2. Certificates
	var certs []models.Certificate
	if err := config.DB.Find(&certs).Error; err == nil {
		for _, c := range certs {
			if key, updated := migrateOne(c.PDFStoragePath, "certificates", "application/pdf"); updated {
				_ = config.DB.Model(&c).Update("pdf_storage_path", key).Error
			}
		}
	}

	// 3. Payment Bills
	var payments []models.Payment
	if err := config.DB.Find(&payments).Error; err == nil {
		for _, p := range payments {
			if key, updated := migrateOne(p.BillPDFPath, "bills", "application/pdf"); updated {
				_ = config.DB.Model(&p).Update("bill_pdf_path", key).Error
			}
		}
	}

	// 4. Research Paper Project Logos / Images
	var papers []models.ResearchPaper
	if err := config.DB.Find(&papers).Error; err == nil {
		for _, rp := range papers {
			if rp.ProjectLogoURL != "" {
				ct := "image/png"
				if strings.HasSuffix(strings.ToLower(rp.ProjectLogoURL), ".jpg") || strings.HasSuffix(strings.ToLower(rp.ProjectLogoURL), ".jpeg") {
					ct = "image/jpeg"
				} else if strings.HasSuffix(strings.ToLower(rp.ProjectLogoURL), ".webp") {
					ct = "image/webp"
				}
				if key, updated := migrateOne(rp.ProjectLogoURL, "logos", ct); updated {
					_ = config.DB.Model(&rp).Update("project_logo_url", key).Error
				}
			}
		}
	}

	log.Printf("S3 Migration Report: scanned=%d uploaded=%d skipped=%d failed=%d",
		report.TotalScanned, report.Uploaded, report.Skipped, report.Failed)
	return report
}
