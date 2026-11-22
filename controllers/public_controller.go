package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"researchhub-backend/config"
	"researchhub-backend/models"
	"researchhub-backend/services"
	"researchhub-backend/utils"

	"github.com/gin-gonic/gin"
)

// GetPublicHome returns real statistics, branding settings, active categories, and recent publications for the home page
func GetPublicHome(c *gin.Context) {
	var totalPublished int64
	var totalSubmissions int64
	var totalReviewers int64
	var totalCategories int64
	var totalCertificates int64

	config.DB.Model(&models.Publication{}).Where("is_published = ?", true).Count(&totalPublished)
	config.DB.Model(&models.ResearchPaper{}).Count(&totalSubmissions)
	config.DB.Model(&models.User{}).Where("role_name = ?", models.RoleSuperAdmin).Count(&totalReviewers)
	config.DB.Model(&models.ResearchCategory{}).Where("is_active = ?", true).Count(&totalCategories)
	config.DB.Model(&models.Certificate{}).Where("is_revoked = ?", false).Count(&totalCertificates)

	var categories []models.ResearchCategory
	config.DB.Where("is_active = ?", true).Order("name ASC").Find(&categories)

	var recentPublications []models.Publication
	config.DB.
		Preload("Paper.Category").
		Preload("Paper.Authors").
		Where("is_published = ?", true).
		Order("publication_date DESC").
		Limit(6).
		Find(&recentPublications)

	var settings []models.SystemSetting
	config.DB.Where("is_public = ?", true).Find(&settings)
	settingsMap := make(map[string]string)
	for _, s := range settings {
		settingsMap[s.SettingKey] = s.SettingValue
	}

	var homeContent models.WebsiteContent
	_ = config.DB.Where("page_key = ?", "home").First(&homeContent).Error

	utils.RespondSuccess(c, http.StatusOK, "Public homepage data retrieved", gin.H{
		"branding": settingsMap,
		"content":  homeContent,
		"stats": gin.H{
			"published_papers":       totalPublished,
			"total_submissions":      totalSubmissions,
			"editorial_reviewers":    totalReviewers,
			"research_categories":    totalCategories,
			"certificates_generated": totalCertificates,
		},
		"categories":          categories,
		"recent_publications": recentPublications,
	})
}

// GetPublicSettings returns centralized branding and journal settings
func GetPublicSettings(c *gin.Context) {
	var settings []models.SystemSetting
	config.DB.Where("is_public = ?", true).Find(&settings)
	settingsMap := make(map[string]string)
	for _, s := range settings {
		settingsMap[s.SettingKey] = s.SettingValue
	}
	utils.RespondSuccess(c, http.StatusOK, "System branding and public settings retrieved", settingsMap)
}

// GetWebsiteContent returns CMS content for public pages
func GetWebsiteContent(c *gin.Context) {
	pageKey := c.Param("pageKey")
	var content models.WebsiteContent
	if err := config.DB.Where("page_key = ?", pageKey).First(&content).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Page content not found", nil)
		return
	}
	utils.RespondSuccess(c, http.StatusOK, "Website content retrieved", content)
}

// GetPublicCategories returns all active research categories
func GetPublicCategories(c *gin.Context) {
	var categories []models.ResearchCategory
	config.DB.Where("is_active = ?", true).Order("name ASC").Find(&categories)
	utils.RespondSuccess(c, http.StatusOK, "Research categories retrieved", categories)
}

// GetEditorialBoard returns Super Admin leadership for the Editorial Board page
func GetEditorialBoard(c *gin.Context) {
	var editors []models.User
	config.DB.Where("role_name = ? AND is_active = ?", models.RoleSuperAdmin, true).Order("full_name ASC").Find(&editors)

	utils.RespondSuccess(c, http.StatusOK, "Editorial board retrieved", gin.H{
		"editors":   editors,
		"reviewers": []models.User{},
	})
}

// SubmitContactMessage saves a public contact inquiry
func SubmitContactMessage(c *gin.Context) {
	var req struct {
		FullName string `json:"full_name" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Phone    string `json:"phone"`
		Subject  string `json:"subject" binding:"required"`
		Message  string `json:"message" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Please complete all required contact fields", err.Error())
		return
	}

	msg := models.ContactMessage{
		FullName: strings.TrimSpace(req.FullName),
		Email:    strings.TrimSpace(req.Email),
		Phone:    strings.TrimSpace(req.Phone),
		Subject:  strings.TrimSpace(req.Subject),
		Message:  strings.TrimSpace(req.Message),
		Status:   "UNREAD",
	}
	if err := config.DB.Create(&msg).Error; err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "Failed to save contact message", err.Error())
		return
	}
	utils.RespondSuccess(c, http.StatusCreated, "Thank you for contacting ResearchHub Editorial Office. We will respond shortly.", msg)
}

// ListPublications handles public search and filtering of published papers
func ListPublications(c *gin.Context) {
	page, limit, offset := utils.GetPaginationParams(c)
	search := strings.TrimSpace(c.Query("search"))
	title := strings.TrimSpace(c.Query("title"))
	author := strings.TrimSpace(c.Query("author"))
	categoryID := strings.TrimSpace(c.Query("category_id"))
	yearStr := strings.TrimSpace(c.Query("year"))

	query := config.DB.Model(&models.Publication{}).
		Preload("Paper.Category").
		Preload("Paper.Authors").
		Where("publications.is_published = ?", true)

	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		query = query.Joins("LEFT JOIN research_papers rp ON rp.id = publications.paper_id").
			Where("LOWER(publications.publication_title) LIKE ? OR LOWER(rp.keywords) LIKE ? OR LOWER(rp.corresponding_author) LIKE ? OR LOWER(publications.publication_code) LIKE ?", like, like, like, like)
	}
	if title != "" {
		like := "%" + strings.ToLower(title) + "%"
		query = query.Where("LOWER(publications.publication_title) LIKE ?", like)
	}
	if author != "" {
		like := "%" + strings.ToLower(author) + "%"
		query = query.Where("publications.paper_id IN (SELECT paper_id FROM paper_authors WHERE LOWER(full_name) LIKE ?)", like)
	}
	if categoryID != "" {
		query = query.Where("publications.paper_id IN (SELECT id FROM research_papers WHERE category_id = ?)", categoryID)
	}
	if yearStr != "" {
		if yr, err := strconv.Atoi(yearStr); err == nil {
			query = query.Where("publications.publication_year = ?", yr)
		}
	}

	var total int64
	query.Count(&total)

	sortBy := strings.ToLower(strings.TrimSpace(c.DefaultQuery("sort_by", "date_desc")))
	orderClause := "publications.publication_date DESC"
	switch sortBy {
	case "date_asc":
		orderClause = "publications.publication_date ASC"
	case "downloads_desc":
		orderClause = "publications.download_count DESC, publications.publication_date DESC"
	case "title_asc":
		orderClause = "publications.publication_title ASC"
	}

	var publications []models.Publication
	query.Order(orderClause).Limit(limit).Offset(offset).Find(&publications)

	utils.RespondPaginated(c, "Published research papers retrieved", publications, page, limit, total)
}

// GetPublicationDetail returns full public details for a single published paper
func GetPublicationDetail(c *gin.Context) {
	id := c.Param("id")
	var pub models.Publication
	err := config.DB.
		Preload("Paper.Category").
		Preload("Paper.Authors").
		Preload("Paper.Files").
		Where("(id = ? OR paper_id = ? OR publication_code = ?) AND is_published = ?", id, id, id, true).
		First(&pub).Error
	if err != nil {
		utils.RespondError(c, http.StatusNotFound, "Published research paper not found", nil)
		return
	}

	_ = config.DB.Model(&pub).Update("view_count", pub.ViewCount+1).Error
	utils.RespondSuccess(c, http.StatusOK, "Publication details retrieved", pub)
}

// DownloadPublishedPDF streams the published PDF file and increments download count
func DownloadPublishedPDF(c *gin.Context) {
	id := c.Param("id")
	var pub models.Publication
	err := config.DB.Preload("Paper.Files").
		Where("(id = ? OR paper_id = ? OR publication_code = ?) AND is_published = ?", id, id, id, true).
		First(&pub).Error
	if err != nil {
		utils.RespondError(c, http.StatusNotFound, "Publication not found", nil)
		return
	}

	var targetFile *models.PaperFile
	if pub.PublishedPDFID != "" {
		var pf models.PaperFile
		if err := config.DB.Where("id = ?", pub.PublishedPDFID).First(&pf).Error; err == nil {
			targetFile = &pf
		}
	}
	if targetFile == nil && pub.Paper != nil && len(pub.Paper.Files) > 0 {
		targetFile = &pub.Paper.Files[0]
	}

	if targetFile == nil {
		utils.RespondError(c, http.StatusNotFound, "Published PDF file not found", nil)
		return
	}

	safePath, err := services.Storage.ResolveSafePath(targetFile.StoragePath)
	if err != nil {
		// Re-generate on the fly if storage directory was cleaned
		storageDir := "./storage"
		if config.Cfg != nil && config.Cfg.StorageLocalPath != "" {
			storageDir = config.Cfg.StorageLocalPath
		}
		if pub.Paper != nil {
			genPath, _, gErr := utils.GenerateSamplePaperPDF(storageDir, pub.Paper.SubmissionCode, pub.PublicationTitle, pub.Paper.CorrespondingAuthor, pub.Paper.Institution, pub.Paper.Abstract, pub.Paper.Keywords)
			if gErr == nil {
				safePath = genPath
				_ = config.DB.Model(targetFile).Update("storage_path", genPath).Error
			}
		}
	}

	if safePath == "" {
		utils.RespondError(c, http.StatusNotFound, "PDF file unavailable on storage", nil)
		return
	}

	_ = config.DB.Model(&pub).Update("download_count", pub.DownloadCount+1).Error
	downloadName := fmt.Sprintf("%s_%s.pdf", pub.PublicationCode, strings.ReplaceAll(pub.PublicationTitle[:minInt(len(pub.PublicationTitle), 30)], " ", "_"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", downloadName))
	c.Header("Content-Type", "application/pdf")
	c.File(safePath)
}

// VerifyCertificatePublic verifies a certificate ID publicly without exposing private student personal contact information
func VerifyCertificatePublic(c *gin.Context) {
	certParam := strings.TrimSpace(c.Param("certificateId"))
	var cert models.Certificate
	err := config.DB.
		Preload("Publication").
		Where("UPPER(certificate_code) = UPPER(?) OR id = ?", certParam, certParam).
		First(&cert).Error

	verificationLog := models.CertificateVerification{
		CertificateCode: certParam,
		VerifiedAt:      time.Now(),
		VerifierIP:      c.ClientIP(),
		UserAgent:       c.GetHeader("User-Agent"),
		IsValidResult:   err == nil && !cert.IsRevoked,
	}
	if err == nil {
		verificationLog.CertificateID = cert.ID
	}
	_ = config.DB.Create(&verificationLog).Error

	if err != nil {
		utils.RespondSuccess(c, http.StatusOK, "Certificate verification completed", gin.H{
			"is_valid":       false,
			"status":         "INVALID",
			"certificate_id": certParam,
			"verified_at":    verificationLog.VerifiedAt,
			"message":        "No valid publication certificate matches the provided Certificate ID.",
		})
		return
	}

	status := "VALID"
	isValid := true
	if cert.IsRevoked {
		status = "REVOKED"
		isValid = false
	}

	qrDataURI, _ := utils.GenerateQRCodeBase64(cert.QRVerificationURL)

	// Strictly return only public academic verification fields (do NOT expose student phone, email, or private profile data)
	utils.RespondSuccess(c, http.StatusOK, "Certificate verification completed", gin.H{
		"is_valid":            isValid,
		"status":              status,
		"certificate_id":      cert.CertificateCode,
		"certificate_holder":  cert.AuthorName,
		"paper_title":         cert.PaperTitle,
		"publication_name":    cert.PublicationName,
		"publication_code":    cert.PublicationCode,
		"volume":              cert.Volume,
		"issue":               cert.Issue,
		"publication_date":    cert.PublicationDate,
		"issued_at":           cert.IssuedAt,
		"signatory_name":      cert.SignatoryName,
		"signatory_title":     cert.SignatoryTitle,
		"qr_verification_url": cert.QRVerificationURL,
		"qr_code_data_uri":    qrDataURI,
		"verified_at":         verificationLog.VerifiedAt,
	})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
