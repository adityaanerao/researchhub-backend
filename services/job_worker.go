package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"researchhub-backend/config"
	"researchhub-backend/models"
	"researchhub-backend/utils"
)

// StartJobWorker launches a persistent background worker for FormattingJob and EmailJob.
func StartJobWorker() {
	// Recover stale PROCESSING/SENDING jobs on startup
	if config.DB != nil {
		_ = config.DB.Model(&models.FormattingJob{}).
			Where("status = ?", models.FormattingProcessing).
			Update("status", models.FormattingQueued).Error
		_ = config.DB.Model(&models.EmailJob{}).
			Where("status = ?", models.EmailSending).
			Update("status", models.EmailQueued).Error
	}

	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			processNextFormattingJob()
			processNextEmailJob()
		}
	}()
	log.Println("[INFO] Persistent PDF Formatting & Email Job Worker started.")
}

func processNextFormattingJob() {
	if config.DB == nil || Storage == nil {
		return
	}
	var job models.FormattingJob
	if err := config.DB.Where("status = ?", models.FormattingQueued).Order("created_at ASC").First(&job).Error; err != nil {
		return
	}
	_ = ExecuteFormattingJob(&job)
}

// ExecuteFormattingJob processes a single FormattingJob synchronously (also used by tests/manual trigger).
func ExecuteFormattingJob(job *models.FormattingJob) error {
	now := time.Now()
	_ = config.DB.Model(job).Updates(map[string]interface{}{
		"status":        models.FormattingProcessing,
		"started_at":    &now,
		"attempt_count": job.AttemptCount + 1,
	}).Error

	var pub models.Publication
	if err := config.DB.Preload("Paper").Where("id = ?", job.PublicationID).First(&pub).Error; err != nil {
		failFormattingJob(job, &pub, "publication record not found")
		return err
	}

	var pf models.PaperFile
	if err := config.DB.Where("id = ?", job.SourceFileID).First(&pf).Error; err != nil {
		failFormattingJob(job, &pub, "source PDF file record not found")
		return err
	}

	reader, _, _, err := Storage.ReadFile(pf.StoragePath)
	if err != nil {
		failFormattingJob(job, &pub, fmt.Sprintf("failed to read source PDF: %v", err))
		return err
	}
	defer reader.Close()

	tmpDir, err := os.MkdirTemp("", "rh_job_*")
	if err != nil {
		failFormattingJob(job, &pub, err.Error())
		return err
	}
	defer os.RemoveAll(tmpDir)

	inPath := filepath.Join(tmpDir, "input.pdf")
	inFile, err := os.Create(inPath)
	if err != nil {
		failFormattingJob(job, &pub, err.Error())
		return err
	}
	_, copyErr := io.Copy(inFile, reader)
	_ = inFile.Close()
	if copyErr != nil {
		failFormattingJob(job, &pub, copyErr.Error())
		return copyErr
	}

	appURL := "https://researchhub.buzz"
	storageDir := "./storage"
	if config.Cfg != nil {
		if config.Cfg.AppURL != "" {
			appURL = config.Cfg.AppURL
		}
		if config.Cfg.StorageLocalPath != "" {
			storageDir = config.Cfg.StorageLocalPath
		}
	}

	// Retrieve official ResearchHub logo via Storage abstraction (S3 or local)
	logoKey := "logo.png"
	var logoSetting models.SystemSetting
	if err := config.DB.Where("setting_key IN ?", []string{"publication_logo", "brand_logo"}).First(&logoSetting).Error; err == nil && logoSetting.SettingValue != "" {
		logoKey = logoSetting.SettingValue
	}
	logoPath := filepath.Join(tmpDir, "logo.png")
	if logoReader, _, _, lErr := Storage.ReadFile(logoKey); lErr == nil {
		if lf, cErr := os.Create(logoPath); cErr == nil {
			_, _ = io.Copy(lf, logoReader)
			_ = lf.Close()
		}
		_ = logoReader.Close()
	} else if localFallback := filepath.Join(storageDir, "logo.png"); func() bool { _, e := os.Stat(localFallback); return e == nil }() {
		logoPath = localFallback
	} else {
		logoPath = ""
	}

	verifyURL := fmt.Sprintf("%s/publications/%s", appURL, pub.PublicationCode)
	opts := utils.DefaultBrandingOptions(pub.PublicationCode, verifyURL, pub.PublicationDate.Format("January 2006"), pub.Volume, pub.Issue, logoPath)
	if job.OptionsJSON != "" {
		_ = json.Unmarshal([]byte(job.OptionsJSON), &opts)
	}
	if opts.HeaderEnabled && opts.LogoPath == "" {
		failFormattingJob(job, &pub, "required official ResearchHub logo is unavailable in storage")
		return fmt.Errorf("required official ResearchHub logo is unavailable")
	}

	outPath := filepath.Join(tmpDir, fmt.Sprintf("%s_branded.pdf", pub.PublicationCode))
	pageCount, err := utils.BrandResearchPaperPDF(inPath, outPath, opts, 200)
	if err != nil {
		failFormattingJob(job, &pub, err.Error())
		return err
	}

	_, objKey, err := Storage.SaveGeneratedFile(outPath, fmt.Sprintf("papers/%s/formatted", pub.PaperID), "application/pdf")
	if err != nil {
		failFormattingJob(job, &pub, fmt.Sprintf("failed to upload branded PDF: %v", err))
		return err
	}

	doneAt := time.Now()
	_ = config.DB.Model(job).Updates(map[string]interface{}{
		"status":           models.FormattingGenerated,
		"output_file_path": objKey,
		"completed_at":     &doneAt,
		"error_message":    "",
	}).Error

	_ = config.DB.Model(&pub).Updates(map[string]interface{}{
		"formatted_file_path":  objKey,
		"formatting_status":    models.FormattingGenerated,
		"formatting_error":     "",
		"original_page_count":  pageCount,
		"formatted_page_count": pageCount,
		"formatted_at":         &doneAt,
	}).Error
	return nil
}

func failFormattingJob(job *models.FormattingJob, pub *models.Publication, msg string) {
	status := models.FormattingFailed
	if job.AttemptCount+1 < job.MaxAttempts {
		status = models.FormattingQueued
	}
	_ = config.DB.Model(job).Updates(map[string]interface{}{
		"status":        status,
		"error_message": msg,
	}).Error
	if pub != nil && pub.ID != "" {
		_ = config.DB.Model(pub).Updates(map[string]interface{}{
			"formatting_status": models.FormattingFailed,
			"formatting_error":  msg,
		}).Error
	}
}

func processNextEmailJob() {
	if config.DB == nil {
		return
	}
	var ej models.EmailJob
	now := time.Now()
	if err := config.DB.Where("status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)", models.EmailQueued, now).
		Order("created_at ASC").First(&ej).Error; err != nil {
		return
	}

	_ = config.DB.Model(&ej).Updates(map[string]interface{}{
		"status":        models.EmailSending,
		"attempt_count": ej.AttemptCount + 1,
	}).Error

	var pub models.Publication
	if err := config.DB.Preload("Paper").Where("id = ?", ej.PublicationID).First(&pub).Error; err != nil {
		_ = config.DB.Model(&ej).Updates(map[string]interface{}{"status": models.EmailFailed, "last_error": "publication not found"}).Error
		return
	}

	attachPath := pub.PublishedFilePath
	if attachPath == "" {
		attachPath = pub.FormattedFilePath
	}
	subject := "Your Research Paper Has Been Published – ResearchHub"
	body := fmt.Sprintf("Congratulations! Your research paper <strong>%s</strong> (%s) has been published on ResearchHub.<br/><br/>View your publication: <a href=\"%s\">%s</a>",
		pub.PublicationTitle, pub.PublicationCode, pub.PublicationURL, pub.PublicationURL)

	SendEmailWithAttachmentAsync(ej.Recipient, ej.Recipient, subject, body, attachPath, fmt.Sprintf("ResearchHub-%s-Research-Paper.pdf", pub.PublicationCode))

	sentAt := time.Now()
	_ = config.DB.Model(&ej).Updates(map[string]interface{}{
		"status":  models.EmailSent,
		"sent_at": &sentAt,
	}).Error
	_ = config.DB.Model(&pub).Updates(map[string]interface{}{
		"email_status":  models.EmailSent,
		"email_sent_at": &sentAt,
	}).Error
}
