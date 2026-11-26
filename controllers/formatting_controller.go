package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"researchhub-backend/config"
	"researchhub-backend/models"
	"researchhub-backend/services"
	"researchhub-backend/utils"

	"github.com/gin-gonic/gin"
)

// FormatPaperRequest configures PDF branding options.
type FormatPaperRequest struct {
	Method                string  `json:"method"`
	HeaderEnabled         *bool   `json:"header_enabled"`
	FooterEnabled         *bool   `json:"footer_enabled"`
	QREnabled             *bool   `json:"qr_enabled"`
	PageNumbersEnabled    *bool   `json:"page_numbers_enabled"`
	WatermarkEnabled      *bool   `json:"watermark_enabled"`
	WatermarkOpacity      float64 `json:"watermark_opacity"`
	AutomaticEmailEnabled *bool   `json:"automatic_email_enabled"`
}

func ensurePublicationForPaper(paperID string, adminID string) (*models.Publication, *models.PaperFile, error) {
	var paper models.ResearchPaper
	if err := config.DB.Preload("Files").Where("id = ?", paperID).First(&paper).Error; err != nil {
		return nil, nil, fmt.Errorf("paper not found")
	}
	if len(paper.Files) == 0 {
		return nil, nil, fmt.Errorf("no uploaded PDF file found for this paper")
	}
	sourceFile := paper.Files[len(paper.Files)-1]

	var pub models.Publication
	if err := config.DB.Where("paper_id = ?", paper.ID).First(&pub).Error; err != nil {
		pubCode := fmt.Sprintf("RHPUB-%d-%s", time.Now().Year(), paper.SubmissionCode)
		pub = models.Publication{
			PublicationCode:  pubCode,
			PaperID:          paper.ID,
			PublicationTitle: paper.Title,
			JournalName:      "International Journal of Academic Research & Innovation",
			Volume:           "12",
			Issue:            "4",
			PublicationYear:  time.Now().Year(),
			PublicationDate:  time.Now(),
			PublicationURL:   fmt.Sprintf("https://researchhub.buzz/publications/%s", pubCode),
			OriginalFileID:   sourceFile.ID,
			FormattingStatus: models.FormattingNotStarted,
			EmailStatus:      models.EmailNotSent,
			PublishedByID:    adminID,
		}
		if cErr := config.DB.Create(&pub).Error; cErr != nil {
			return nil, nil, cErr
		}
	}
	return &pub, &sourceFile, nil
}

// AdminFormatPaper queues a background PDF branding job for a paper.
func AdminFormatPaper(c *gin.Context) {
	paperID := c.Param("id")
	adminID := c.GetString("user_id")

	var req FormatPaperRequest
	_ = c.ShouldBindJSON(&req)

	pub, sourceFile, err := ensurePublicationForPaper(paperID, adminID)
	if err != nil {
		utils.RespondError(c, http.StatusNotFound, err.Error(), nil)
		return
	}
	if pub.IsPublished {
		utils.RespondError(c, http.StatusConflict, "Cannot reformat an already published paper; published PDFs are immutable", nil)
		return
	}

	var activeJob models.FormattingJob
	if err := config.DB.Where("paper_id = ? AND status IN ?", pub.PaperID, []string{models.FormattingQueued, models.FormattingProcessing}).First(&activeJob).Error; err == nil {
		utils.RespondError(c, http.StatusConflict, "A formatting job is already queued or processing for this paper", nil)
		return
	}

	verifyURL := fmt.Sprintf("https://researchhub.buzz/publications/%s", pub.PublicationCode)
	opts := utils.DefaultBrandingOptions(pub.PublicationCode, verifyURL, pub.PublicationDate.Format("January 2006"), pub.Volume, pub.Issue, "")
	if req.HeaderEnabled != nil {
		opts.HeaderEnabled = *req.HeaderEnabled
	}
	if req.FooterEnabled != nil {
		opts.FooterEnabled = *req.FooterEnabled
	}
	if req.QREnabled != nil {
		opts.QREnabled = *req.QREnabled
	}
	if req.PageNumbersEnabled != nil {
		opts.PageNumbers = *req.PageNumbersEnabled
	}
	if req.WatermarkEnabled != nil {
		opts.WatermarkEnabled = *req.WatermarkEnabled
	}
	if req.WatermarkOpacity > 0 {
		opts.WatermarkOpacity = req.WatermarkOpacity
	}
	optsBytes, _ := json.Marshal(opts)

	job := models.FormattingJob{
		PaperID:       pub.PaperID,
		PublicationID: pub.ID,
		SourceFileID:  sourceFile.ID,
		Status:        models.FormattingQueued,
		OptionsJSON:   string(optsBytes),
		MaxAttempts:   3,
	}
	if err := config.DB.Create(&job).Error; err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "Failed to queue formatting job", err.Error())
		return
	}

	_ = config.DB.Model(pub).Updates(map[string]interface{}{
		"formatting_status": models.FormattingQueued,
		"formatting_error":  "",
		"formatted_by":      adminID,
	}).Error

	utils.RespondSuccess(c, http.StatusAccepted, "Branded PDF generation queued", gin.H{
		"job_id":            job.ID,
		"publication_id":    pub.ID,
		"publication_code":  pub.PublicationCode,
		"formatting_status": models.FormattingQueued,
	})
}

// AdminGetFormatStatus returns the current PDF formatting status for a paper.
func AdminGetFormatStatus(c *gin.Context) {
	paperID := c.Param("id")
	var pub models.Publication
	if err := config.DB.Where("paper_id = ?", paperID).First(&pub).Error; err != nil {
		var paper models.ResearchPaper
		if pErr := config.DB.Where("id = ?", paperID).First(&paper).Error; pErr != nil {
			utils.RespondError(c, http.StatusNotFound, "Research paper not found", nil)
			return
		}
		utils.RespondSuccess(c, http.StatusOK, "Formatting status retrieved", gin.H{
			"paper_id":          paper.ID,
			"formatting_status": models.FormattingNotStarted,
			"email_status":      models.EmailNotSent,
		})
		return
	}
	utils.RespondSuccess(c, http.StatusOK, "Formatting status retrieved", pub)
}

// AdminPreviewFormattedPDF streams the generated branded PDF for admin preview.
func AdminPreviewFormattedPDF(c *gin.Context) {
	paperID := c.Param("id")
	var pub models.Publication
	if err := config.DB.Where("paper_id = ?", paperID).First(&pub).Error; err != nil || pub.FormattedFilePath == "" {
		utils.RespondError(c, http.StatusNotFound, "Formatted PDF not available yet", nil)
		return
	}
	reader, ct, size, err := services.Storage.ReadFile(pub.FormattedFilePath)
	if err != nil {
		utils.RespondError(c, http.StatusNotFound, "Formatted PDF unavailable on storage", nil)
		return
	}
	defer reader.Close()
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s_preview.pdf\"", pub.PublicationCode))
	c.DataFromReader(http.StatusOK, size, ct, reader, nil)
}

// AdminDownloadOriginalPDF streams the original uploaded PDF.
func AdminDownloadOriginalPDF(c *gin.Context) {
	paperID := c.Param("id")
	var pf models.PaperFile
	if err := config.DB.Where("paper_id = ?", paperID).Order("created_at DESC").First(&pf).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Original PDF not found", nil)
		return
	}
	reader, ct, size, err := services.Storage.ReadFile(pf.StoragePath)
	if err != nil {
		utils.RespondError(c, http.StatusNotFound, "Original PDF unavailable on storage", nil)
		return
	}
	defer reader.Close()
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", pf.OriginalName))
	c.DataFromReader(http.StatusOK, size, ct, reader, nil)
}

// AdminDownloadFormattedPDF streams the generated branded PDF as an attachment.
func AdminDownloadFormattedPDF(c *gin.Context) {
	paperID := c.Param("id")
	var pub models.Publication
	if err := config.DB.Where("paper_id = ?", paperID).First(&pub).Error; err != nil || pub.FormattedFilePath == "" {
		utils.RespondError(c, http.StatusNotFound, "Formatted PDF not available", nil)
		return
	}
	reader, ct, size, err := services.Storage.ReadFile(pub.FormattedFilePath)
	if err != nil {
		utils.RespondError(c, http.StatusNotFound, "Formatted PDF unavailable on storage", nil)
		return
	}
	defer reader.Close()
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"ResearchHub-%s-Research-Paper.pdf\"", pub.PublicationCode))
	c.DataFromReader(http.StatusOK, size, ct, reader, nil)
}

// AdminApproveFormattedPDF marks the generated branded PDF as APPROVED for publication.
func AdminApproveFormattedPDF(c *gin.Context) {
	paperID := c.Param("id")
	adminID := c.GetString("user_id")
	var pub models.Publication
	if err := config.DB.Where("paper_id = ?", paperID).First(&pub).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Publication record not found", nil)
		return
	}
	if pub.FormattedFilePath == "" || pub.FormattingStatus != models.FormattingGenerated {
		utils.RespondError(c, http.StatusBadRequest, "Formatted PDF must be generated before approval", nil)
		return
	}
	now := time.Now()
	_ = config.DB.Model(&pub).Updates(map[string]interface{}{
		"formatting_status":      models.FormattingApproved,
		"published_file_path":    pub.FormattedFilePath,
		"formatting_approved_at": &now,
		"formatting_approved_by": adminID,
	}).Error

	utils.RespondSuccess(c, http.StatusOK, "Formatted PDF approved", gin.H{
		"publication_id":    pub.ID,
		"formatting_status": models.FormattingApproved,
	})
}

// AdminGetEmailStatus returns email delivery status for a publication.
func AdminGetEmailStatus(c *gin.Context) {
	pubID := c.Param("id")
	var pub models.Publication
	if err := config.DB.Where("id = ?", pubID).First(&pub).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Publication not found", nil)
		return
	}
	utils.RespondSuccess(c, http.StatusOK, "Email status retrieved", gin.H{
		"publication_id":    pub.ID,
		"email_status":      pub.EmailStatus,
		"email_sent_at":     pub.EmailSentAt,
		"email_retry_count": pub.EmailRetryCount,
		"email_last_error":  pub.EmailLastError,
	})
}

// AdminResendPublicationEmail queues a new EmailJob for a published paper.
func AdminResendPublicationEmail(c *gin.Context) {
	pubID := c.Param("id")
	var pub models.Publication
	if err := config.DB.Preload("Paper").Where("id = ?", pubID).First(&pub).Error; err != nil || pub.Paper == nil {
		utils.RespondError(c, http.StatusNotFound, "Publication not found", nil)
		return
	}
	ej := models.EmailJob{
		PublicationID: pub.ID,
		PaperID:       pub.PaperID,
		Recipient:     pub.Paper.CorrespondingEmail,
		Status:        models.EmailQueued,
		MaxAttempts:   3,
	}
	if err := config.DB.Create(&ej).Error; err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "Failed to queue email job", err.Error())
		return
	}
	_ = config.DB.Model(&pub).Update("email_status", models.EmailQueued).Error
	utils.RespondSuccess(c, http.StatusAccepted, "Publication email queued for delivery", gin.H{"job_id": ej.ID})
}
