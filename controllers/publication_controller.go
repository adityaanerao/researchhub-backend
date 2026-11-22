package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"researchhub-backend/config"
	"researchhub-backend/models"
	"researchhub-backend/repositories"
	"researchhub-backend/services"
	"researchhub-backend/utils"

	"github.com/gin-gonic/gin"
)

// ListAdminPublications returns all publication records (draft/ready and published) for Editors and Super Admins
func ListAdminPublications(c *gin.Context) {
	var publications []models.Publication
	config.DB.
		Preload("Paper.Category").
		Preload("Paper.Authors").
		Preload("Paper.Files").
		Order("created_at DESC").
		Find(&publications)

	var eligiblePapers []models.ResearchPaper
	repositories.PreloadPaperFull(config.DB).
		Where("status IN ?", []string{models.StatusApproved, models.StatusReadyForPublication}).
		Order("updated_at DESC").
		Find(&eligiblePapers)

	var certificates []models.Certificate
	config.DB.Order("issued_at DESC").Find(&certificates)

	utils.RespondSuccess(c, http.StatusOK, "Publication management records retrieved", gin.H{
		"publications":    publications,
		"eligible_papers": eligiblePapers,
		"certificates":    certificates,
	})
}

// CreateOrPreparePublication creates a publication record for an approved paper and optionally publishes it immediately
func CreateOrPreparePublication(c *gin.Context) {
	actorID := c.GetString("user_id")
	actorName := c.GetString("user_name")
	actorEmail := c.GetString("user_email")
	actorRole := c.GetString("user_role")

	// Support both JSON and multipart/form-data (in case a final formatted PDF is uploaded)
	var paperID, publicationTitle, journalName, volume, issue, doi, issn, pubDateStr string
	var publicationYear, pageStart, pageEnd int
	var publishNow bool

	contentType := c.GetHeader("Content-Type")
	if strings.Contains(contentType, "multipart/form-data") {
		paperID = strings.TrimSpace(c.PostForm("paper_id"))
		publicationTitle = strings.TrimSpace(c.PostForm("publication_title"))
		journalName = strings.TrimSpace(c.PostForm("journal_name"))
		volume = strings.TrimSpace(c.PostForm("volume"))
		issue = strings.TrimSpace(c.PostForm("issue"))
		doi = strings.TrimSpace(c.PostForm("doi"))
		issn = strings.TrimSpace(c.PostForm("issn"))
		pubDateStr = strings.TrimSpace(c.PostForm("publication_date"))
		publicationYear, _ = strconv.Atoi(c.DefaultPostForm("publication_year", fmt.Sprint(time.Now().Year())))
		pageStart, _ = strconv.Atoi(c.DefaultPostForm("page_start", "1"))
		pageEnd, _ = strconv.Atoi(c.DefaultPostForm("page_end", "10"))
		publishNow = c.DefaultPostForm("publish_now", "true") == "true"
	} else {
		var req struct {
			PaperID          string `json:"paper_id" binding:"required"`
			PublicationTitle string `json:"publication_title"`
			JournalName      string `json:"journal_name"`
			Volume           string `json:"volume" binding:"required"`
			Issue            string `json:"issue" binding:"required"`
			PublicationYear  int    `json:"publication_year"`
			PublicationDate  string `json:"publication_date"`
			DOI              string `json:"doi"` // Optional, never generated falsely
			ISSN             string `json:"issn"`
			PageStart        int    `json:"page_start"`
			PageEnd          int    `json:"page_end"`
			PublishNow       *bool  `json:"publish_now"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.RespondError(c, http.StatusBadRequest, "Paper ID, Volume, and Issue are required for publication", err.Error())
			return
		}
		paperID = strings.TrimSpace(req.PaperID)
		publicationTitle = strings.TrimSpace(req.PublicationTitle)
		journalName = strings.TrimSpace(req.JournalName)
		volume = strings.TrimSpace(req.Volume)
		issue = strings.TrimSpace(req.Issue)
		doi = strings.TrimSpace(req.DOI)
		issn = strings.TrimSpace(req.ISSN)
		pubDateStr = strings.TrimSpace(req.PublicationDate)
		publicationYear = req.PublicationYear
		pageStart = req.PageStart
		pageEnd = req.PageEnd
		publishNow = true
		if req.PublishNow != nil {
			publishNow = *req.PublishNow
		}
	}

	var paper models.ResearchPaper
	if err := repositories.PreloadPaperFull(config.DB).Where("id = ?", paperID).First(&paper).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Research paper not found", nil)
		return
	}

	// Strictly enforce rule: Publish only approved papers (or READY_FOR_PUBLICATION / already PUBLISHED update)
	if paper.Status != models.StatusApproved && paper.Status != models.StatusReadyForPublication && paper.Status != models.StatusPublished {
		utils.RespondError(c, http.StatusBadRequest, fmt.Sprintf("Only APPROVED or READY_FOR_PUBLICATION papers can be published (current status: %s)", paper.Status), nil)
		return
	}

	if publicationTitle == "" {
		publicationTitle = paper.Title
	}
	if journalName == "" {
		journalName = "ResearchHub Transactions on Multidisciplinary Sciences"
	}
	if publicationYear <= 0 {
		publicationYear = time.Now().Year()
	}
	if pageStart <= 0 {
		pageStart = 1
	}
	if pageEnd < pageStart {
		pageEnd = pageStart + 9
	}

	pubDate := time.Now()
	if pubDateStr != "" {
		if parsed, err := time.Parse("2006-01-02", pubDateStr); err == nil {
			pubDate = parsed
		}
	}

	// Check if a final published PDF file was uploaded; otherwise use latest paper file
	publishedPDFID := ""
	if len(paper.Files) > 0 {
		publishedPDFID = paper.Files[0].ID
	}
	if fileHeader, err := c.FormFile("pdf_file"); err == nil && fileHeader != nil {
		storedName, fullPath, fileSize, sErr := services.Storage.SavePDFFile(fileHeader, "publications")
		if sErr != nil {
			utils.RespondError(c, http.StatusBadRequest, sErr.Error(), nil)
			return
		}
		pf := models.PaperFile{
			PaperID:        paper.ID,
			FileType:       "PUBLISHED_PDF",
			OriginalName:   fileHeader.Filename,
			StoredFileName: storedName,
			StoragePath:    fullPath,
			MimeType:       "application/pdf",
			FileSize:       fileSize,
			VersionNumber:  len(paper.Files) + 1,
			UploadedByID:   actorID,
		}
		if err := config.DB.Create(&pf).Error; err == nil {
			publishedPDFID = pf.ID
		}
	}

	var pub models.Publication
	isNew := false
	if err := config.DB.Where("paper_id = ?", paper.ID).First(&pub).Error; err != nil {
		isNew = true
		var count int64
		config.DB.Model(&models.Publication{}).Unscoped().Count(&count)
		pubCode := fmt.Sprintf("RHPUB-%d-%03d", publicationYear, count+1)
		pub = models.Publication{
			PublicationCode: pubCode,
			PaperID:         paper.ID,
		}
	}

	pub.PublicationTitle = publicationTitle
	pub.JournalName = journalName
	pub.Volume = volume
	pub.Issue = issue
	pub.PublicationYear = publicationYear
	pub.PublicationDate = pubDate
	pub.DOI = doi // Stored ONLY when explicitly provided, never faked!
	pub.ISSN = issn
	pub.PageStart = pageStart
	pub.PageEnd = pageEnd
	pub.PublishedPDFID = publishedPDFID
	pub.PublicationURL = "/publications/" + paper.ID
	pub.PublishedByID = actorID

	if isNew {
		if err := config.DB.Create(&pub).Error; err != nil {
			utils.RespondError(c, http.StatusInternalServerError, "Failed to create publication record", err.Error())
			return
		}
	} else {
		_ = config.DB.Save(&pub).Error
	}

	if publishNow {
		certs, err := finalizePublicationAndCertificates(&pub, &paper, actorID, actorName, actorEmail, actorRole, c.ClientIP())
		if err != nil {
			utils.RespondError(c, http.StatusInternalServerError, "Publication saved but certificate generation encountered an error", err.Error())
			return
		}
		utils.RespondSuccess(c, http.StatusCreated, "Research paper published and certificates generated automatically", gin.H{
			"publication":  pub,
			"certificates": certs,
		})
		return
	}

	// Otherwise move paper to READY_FOR_PUBLICATION
	oldStatus := paper.Status
	_ = config.DB.Model(&paper).Update("status", models.StatusReadyForPublication).Error
	_ = repositories.RecordPaperStatusChange(config.DB, paper.ID, oldStatus, models.StatusReadyForPublication, actorID, actorName, actorRole, "Publication metadata prepared.")

	utils.RespondSuccess(c, http.StatusCreated, "Publication record prepared and marked READY_FOR_PUBLICATION", gin.H{
		"publication": pub,
	})
}

// PublishPreparedRecord finalizes publication for an existing publication ID and generates author certificates
func PublishPreparedRecord(c *gin.Context) {
	actorID := c.GetString("user_id")
	actorName := c.GetString("user_name")
	actorEmail := c.GetString("user_email")
	actorRole := c.GetString("user_role")
	pubID := c.Param("id")

	var pub models.Publication
	if err := config.DB.Where("id = ? OR paper_id = ?", pubID, pubID).First(&pub).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Publication record not found", nil)
		return
	}

	var paper models.ResearchPaper
	if err := repositories.PreloadPaperFull(config.DB).Where("id = ?", pub.PaperID).First(&paper).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Associated research paper not found", nil)
		return
	}

	if paper.Status != models.StatusApproved && paper.Status != models.StatusReadyForPublication && paper.Status != models.StatusPublished {
		utils.RespondError(c, http.StatusBadRequest, "Only approved papers can be published", nil)
		return
	}

	certs, err := finalizePublicationAndCertificates(&pub, &paper, actorID, actorName, actorEmail, actorRole, c.ClientIP())
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "Failed to finalize publication", err.Error())
		return
	}

	utils.RespondSuccess(c, http.StatusOK, "Research paper published and author certificates generated", gin.H{
		"publication":  pub,
		"certificates": certs,
	})
}

func finalizePublicationAndCertificates(pub *models.Publication, paper *models.ResearchPaper, actorID, actorName, actorEmail, actorRole, ip string) ([]models.Certificate, error) {
	now := time.Now()
	oldStatus := paper.Status

	pub.IsPublished = true
	pub.PublishedByID = actorID
	if pub.PublicationDate.IsZero() {
		pub.PublicationDate = now
	}
	_ = config.DB.Save(pub).Error

	_ = config.DB.Model(paper).Updates(map[string]interface{}{
		"status":       models.StatusPublished,
		"published_at": now,
	}).Error

	_ = repositories.RecordPaperStatusChange(
		config.DB,
		paper.ID,
		oldStatus,
		models.StatusPublished,
		actorID,
		actorName,
		actorRole,
		fmt.Sprintf("Published in %s (Volume %s, Issue %s) with Publication ID %s.", pub.JournalName, pub.Volume, pub.Issue, pub.PublicationCode),
	)

	// Load signatory settings
	signatoryName := "Prof. Dr. Eleanor Vance"
	signatoryTitle := "Editor-in-Chief, ResearchHub Editorial Board"
	var sName, sTitle models.SystemSetting
	if err := config.DB.Where("setting_key = ?", "signatory_name").First(&sName).Error; err == nil && sName.SettingValue != "" {
		signatoryName = sName.SettingValue
	}
	if err := config.DB.Where("setting_key = ?", "signatory_title").First(&sTitle).Error; err == nil && sTitle.SettingValue != "" {
		signatoryTitle = sTitle.SettingValue
	}

	storageDir := "./storage"
	appURL := "http://localhost:4200"
	if config.Cfg != nil {
		if config.Cfg.StorageLocalPath != "" {
			storageDir = config.Cfg.StorageLocalPath
		}
		if config.Cfg.AppURL != "" {
			appURL = config.Cfg.AppURL
		}
	}

	authors := paper.Authors
	if len(authors) == 0 {
		authors = []models.PaperAuthor{
			{
				PaperID:     paper.ID,
				FullName:    paper.CorrespondingAuthor,
				Email:       paper.CorrespondingEmail,
				Institution: paper.Institution,
				AuthorOrder: 1,
			},
		}
	}

	var generatedCerts []models.Certificate
	for idx, author := range authors {
		certCode := fmt.Sprintf("RHCERT-%s-A%d", strings.TrimPrefix(paper.SubmissionCode, "RH-"), idx+1)
		var existingCert models.Certificate
		if err := config.DB.Where("certificate_code = ?", certCode).First(&existingCert).Error; err == nil {
			generatedCerts = append(generatedCerts, existingCert)
			continue
		}

		verifyURL := fmt.Sprintf("%s/verify-certificate/%s", appURL, certCode)
		pdfPath, _ := utils.GenerateCertificatePDF(
			storageDir,
			certCode,
			"ResearchHub",
			pub.JournalName,
			author.FullName,
			author.Institution,
			pub.PublicationTitle,
			pub.PublicationCode,
			pub.DOI,
			pub.Volume,
			pub.Issue,
			pub.PublicationDate.Format("January 02, 2006"),
			signatoryName,
			signatoryTitle,
			verifyURL,
		)

		cert := models.Certificate{
			CertificateCode:   certCode,
			PublicationID:     &pub.ID,
			PaperID:           paper.ID,
			StudentID:         paper.StudentID,
			AuthorName:        author.FullName,
			AuthorEmail:       author.Email,
			PaperTitle:        pub.PublicationTitle,
			PublicationName:   pub.JournalName,
			PublicationCode:   pub.PublicationCode,
			Volume:            pub.Volume,
			Issue:             pub.Issue,
			PublicationDate:   pub.PublicationDate,
			SignatoryName:     signatoryName,
			SignatoryTitle:    signatoryTitle,
			QRVerificationURL: verifyURL,
			PDFStoragePath:    pdfPath,
			IsRevoked:         false,
			IssuedAt:          now,
		}
		if err := config.DB.Create(&cert).Error; err == nil {
			generatedCerts = append(generatedCerts, cert)
		}
	}

	// Send notifications for publication and certificate generation
	services.CreateNotification(
		paper.StudentID,
		"Paper Published: "+pub.PublicationCode,
		fmt.Sprintf("Congratulations! Your research paper '%s' has been published in %s (Volume %s, Issue %s).", pub.PublicationTitle, pub.JournalName, pub.Volume, pub.Issue),
		"PUBLICATION",
		paper.ID,
	)
	services.CreateNotification(
		paper.StudentID,
		"Publication Certificate(s) Generated",
		fmt.Sprintf("%d official PDF publication certificate(s) with QR verification have been generated for '%s'.", len(generatedCerts), pub.PublicationTitle),
		"CERTIFICATE",
		paper.ID,
	)

	services.LogAudit(actorID, actorEmail, actorRole, "PAPER_PUBLISHED", "publications", pub.ID, "Published "+pub.PublicationCode, ip)
	return generatedCerts, nil
}
