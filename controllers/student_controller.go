package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"researchhub-backend/config"
	"researchhub-backend/models"
	"researchhub-backend/repositories"
	"researchhub-backend/services"
	"researchhub-backend/utils"

	"github.com/gin-gonic/gin"
)

// GetStudentDashboard returns live database metrics and recent submissions for the logged-in student
func GetStudentDashboard(c *gin.Context) {
	studentID := c.GetString("user_id")

	var totalSubmissions, underReview, revisionRequired, approved, published, rejected int64

	config.DB.Model(&models.ResearchPaper{}).Where("student_id = ?", studentID).Count(&totalSubmissions)
	config.DB.Model(&models.ResearchPaper{}).Where("student_id = ? AND status IN ?", studentID, []string{
		models.StatusSubmitted,
		models.StatusInitialScreening,
		models.StatusUnderReview,
		models.StatusRevisionSubmitted,
		models.StatusReviewRecommendedApproval,
		models.StatusReviewRecommendedRejection,
		models.StatusFinalEditorialDecision,
	}).Count(&underReview)
	config.DB.Model(&models.ResearchPaper{}).Where("student_id = ? AND status = ?", studentID, models.StatusRevisionRequired).Count(&revisionRequired)
	config.DB.Model(&models.ResearchPaper{}).Where("student_id = ? AND status IN ?", studentID, []string{
		models.StatusApproved,
		models.StatusReadyForPublication,
	}).Count(&approved)
	config.DB.Model(&models.ResearchPaper{}).Where("student_id = ? AND status = ?", studentID, models.StatusPublished).Count(&published)
	config.DB.Model(&models.ResearchPaper{}).Where("student_id = ? AND status = ?", studentID, models.StatusRejected).Count(&rejected)

	var recentPapers []models.ResearchPaper
	repositories.PreloadPaperFull(config.DB).
		Where("student_id = ?", studentID).
		Order("created_at DESC").
		Limit(10).
		Find(&recentPapers)

	// Sanitize confidential reviewer comments before sending to student
	for i := range recentPapers {
		sanitizePaperForStudent(&recentPapers[i])
	}

	var certCount int64
	config.DB.Model(&models.Certificate{}).
		Where("student_id = ? OR paper_id IN (SELECT id FROM research_papers WHERE student_id = ?)", studentID, studentID).
		Count(&certCount)

	var notifications []models.Notification
	config.DB.Where("user_id = ?", studentID).Order("created_at DESC").Limit(8).Find(&notifications)

	utils.RespondSuccess(c, http.StatusOK, "Student dashboard retrieved", gin.H{
		"cards": gin.H{
			"total_submissions": totalSubmissions,
			"under_review":      underReview,
			"revision_required": revisionRequired,
			"approved":          approved,
			"published":         published,
			"rejected":          rejected,
			"certificates":      certCount,
		},
		"recent_papers": recentPapers,
		"notifications": notifications,
	})
}

// GetStudentProfile returns the student's user and academic profile
func GetStudentProfile(c *gin.Context) {
	studentID := c.GetString("user_id")
	var user models.User
	if err := config.DB.Preload("StudentProfile").Where("id = ?", studentID).First(&user).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Student profile not found", nil)
		return
	}
	utils.RespondSuccess(c, http.StatusOK, "Student profile retrieved", user)
}

// UpdateStudentProfile updates the student's user and academic profile
func UpdateStudentProfile(c *gin.Context) {
	studentID := c.GetString("user_id")
	var req struct {
		FullName         string `json:"full_name" binding:"required"`
		Phone            string `json:"phone"`
		Institution      string `json:"institution" binding:"required"`
		Department       string `json:"department"`
		Designation      string `json:"designation"`
		Bio              string `json:"bio"`
		EnrollmentNumber string `json:"enrollment_number"`
		DegreeProgram    string `json:"degree_program"`
		AcademicYear     string `json:"academic_year"`
		GuideName        string `json:"guide_name"`
		GuideEmail       string `json:"guide_email"`
		ResearchInterest string `json:"research_interest"`
		ORCID            string `json:"orcid"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Validation failed: full name and institution are required", err.Error())
		return
	}

	var user models.User
	if err := config.DB.Where("id = ?", studentID).First(&user).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "User not found", nil)
		return
	}

	_ = config.DB.Model(&user).Updates(map[string]interface{}{
		"full_name":   strings.TrimSpace(req.FullName),
		"phone":       strings.TrimSpace(req.Phone),
		"institution": strings.TrimSpace(req.Institution),
		"department":  strings.TrimSpace(req.Department),
		"designation": strings.TrimSpace(req.Designation),
		"bio":         strings.TrimSpace(req.Bio),
	}).Error

	var profile models.StudentProfile
	if err := config.DB.Where("user_id = ?", studentID).First(&profile).Error; err != nil {
		profile = models.StudentProfile{UserID: studentID}
		_ = config.DB.Create(&profile).Error
	}
	_ = config.DB.Model(&profile).Updates(map[string]interface{}{
		"enrollment_number": strings.TrimSpace(req.EnrollmentNumber),
		"degree_program":    strings.TrimSpace(req.DegreeProgram),
		"academic_year":     strings.TrimSpace(req.AcademicYear),
		"institution":       strings.TrimSpace(req.Institution),
		"department":        strings.TrimSpace(req.Department),
		"guide_name":        strings.TrimSpace(req.GuideName),
		"guide_email":       strings.TrimSpace(req.GuideEmail),
		"research_interest": strings.TrimSpace(req.ResearchInterest),
		"orcid":             strings.TrimSpace(req.ORCID),
	}).Error

	_ = config.DB.Preload("StudentProfile").Where("id = ?", studentID).First(&user).Error
	services.LogAudit(user.ID, user.Email, user.RoleName, "STUDENT_PROFILE_UPDATED", "users", user.ID, "Updated student profile", c.ClientIP())
	utils.RespondSuccess(c, http.StatusOK, "Profile updated successfully", user)
}

type AuthorInput struct {
	FullName        string `json:"full_name"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	Institution     string `json:"institution"`
	Department      string `json:"department"`
	Designation     string `json:"designation"`
	AuthorOrder     int    `json:"author_order"`
	IsCorresponding bool   `json:"is_corresponding"`
}

// SubmitResearchPaper handles new research paper submission with multi-author support and PDF upload
func SubmitResearchPaper(c *gin.Context) {
	studentID := c.GetString("user_id")
	studentName := c.GetString("user_name")
	studentEmail := c.GetString("user_email")

	title := strings.TrimSpace(c.PostForm("title"))
	abstract := strings.TrimSpace(c.PostForm("abstract"))
	keywords := strings.TrimSpace(c.PostForm("keywords"))
	categoryID := strings.TrimSpace(c.PostForm("category_id"))
	researchArea := strings.TrimSpace(c.PostForm("research_area"))
	correspondingAuthor := strings.TrimSpace(c.PostForm("corresponding_author"))
	correspondingEmail := strings.TrimSpace(c.PostForm("corresponding_email"))
	contactNumber := strings.TrimSpace(c.PostForm("contact_number"))
	institution := strings.TrimSpace(c.PostForm("institution"))
	department := strings.TrimSpace(c.PostForm("department"))
	guideName := strings.TrimSpace(c.PostForm("guide_name"))
	declarationAccepted := c.PostForm("declaration_accepted") != "false"
	originalityConfirmed := c.PostForm("originality_confirmed") != "false"
	requestedStatus := strings.ToUpper(strings.TrimSpace(c.PostForm("status")))
	authorsJSON := c.PostForm("authors")
	customCategoryName := strings.TrimSpace(c.PostForm("custom_category_name"))

	if (categoryID == "" || categoryID == "__OTHER__") && customCategoryName != "" {
		var existingCat models.ResearchCategory
		if err := config.DB.Where("LOWER(name) = LOWER(?)", customCategoryName).First(&existingCat).Error; err == nil {
			categoryID = existingCat.ID
		} else {
			code := strings.ToUpper(strings.ReplaceAll(customCategoryName, " ", ""))
			if len(code) > 6 {
				code = code[:6]
			}
			code = fmt.Sprintf("%s%d", code, time.Now().Unix()%1000)
			newCat := models.ResearchCategory{
				Name:        customCategoryName,
				Code:        code,
				Description: "Custom research category added during paper submission",
				IsActive:    true,
			}
			if cErr := config.DB.Create(&newCat).Error; cErr == nil {
				categoryID = newCat.ID
			}
		}
	}

	if title == "" || abstract == "" || categoryID == "" || categoryID == "__OTHER__" || institution == "" {
		utils.RespondError(c, http.StatusBadRequest, "Title, Abstract, Research Category, and Institution are required", nil)
		return
	}
	if !declarationAccepted || !originalityConfirmed {
		utils.RespondError(c, http.StatusBadRequest, "You must accept the declaration and confirm originality of the research manuscript", nil)
		return
	}
	if correspondingAuthor == "" {
		correspondingAuthor = studentName
	}
	if correspondingEmail == "" {
		correspondingEmail = studentEmail
	}

	initialStatus := models.StatusSubmitted
	if requestedStatus == models.StatusDraft {
		initialStatus = models.StatusDraft
	}

	fileHeader, err := c.FormFile("pdf_file")
	if err != nil || fileHeader == nil {
		utils.RespondError(c, http.StatusBadRequest, "A valid Research Paper PDF file is required", nil)
		return
	}

	storedName, fullPath, fileSize, err := services.Storage.SavePDFFile(fileHeader, "papers")
	if err != nil {
		utils.RespondError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	projectLogoURL := ""
	if logoHeader, lErr := c.FormFile("project_logo"); lErr == nil && logoHeader != nil {
		if logoName, _, sErr := services.Storage.SaveImageFile(logoHeader, "logos"); sErr == nil {
			projectLogoURL = "/api/v1/public/logos/" + logoName
		}
	}

	// Generate unique paper submission code
	var count int64
	config.DB.Model(&models.ResearchPaper{}).Unscoped().Count(&count)
	submissionCode := fmt.Sprintf("RH-%d-%04d", time.Now().Year(), 1001+count)
	// Ensure uniqueness
	var existingCode models.ResearchPaper
	if err := config.DB.Where("submission_code = ?", submissionCode).First(&existingCode).Error; err == nil {
		submissionCode = fmt.Sprintf("RH-%d-%04d-%s", time.Now().Year(), 1001+count, strings.ToUpper(utils.GenerateSecureToken(2)))
	}

	tx := config.DB.Begin()

	paper := models.ResearchPaper{
		SubmissionCode:       submissionCode,
		Title:                title,
		Abstract:             abstract,
		Keywords:             keywords,
		CategoryID:           categoryID,
		ResearchArea:         researchArea,
		StudentID:            studentID,
		CorrespondingAuthor:  correspondingAuthor,
		CorrespondingEmail:   correspondingEmail,
		ContactNumber:        contactNumber,
		Institution:          institution,
		Department:           department,
		GuideName:            guideName,
		DeclarationAccepted:  declarationAccepted,
		OriginalityConfirmed: originalityConfirmed,
		ProjectLogoURL:       projectLogoURL,
		Status:               initialStatus,
		CurrentRound:         1,
		SubmittedAt:          time.Now(),
	}

	if err := tx.Create(&paper).Error; err != nil {
		tx.Rollback()
		utils.RespondError(c, http.StatusInternalServerError, "Failed to save research paper submission", err.Error())
		return
	}

	// Parse authors array
	var authorInputs []AuthorInput
	if authorsJSON != "" {
		_ = json.Unmarshal([]byte(authorsJSON), &authorInputs)
	}
	if len(authorInputs) == 0 {
		authorInputs = append(authorInputs, AuthorInput{
			FullName:        correspondingAuthor,
			Email:           correspondingEmail,
			Phone:           contactNumber,
			Institution:     institution,
			Department:      department,
			Designation:     "Corresponding Author",
			AuthorOrder:     1,
			IsCorresponding: true,
		})
	}

	for idx, a := range authorInputs {
		if strings.TrimSpace(a.FullName) == "" {
			continue
		}
		order := a.AuthorOrder
		if order <= 0 {
			order = idx + 1
		}
		authorRec := models.PaperAuthor{
			PaperID:         paper.ID,
			FullName:        strings.TrimSpace(a.FullName),
			Email:           strings.TrimSpace(a.Email),
			Phone:           strings.TrimSpace(a.Phone),
			Institution:     strings.TrimSpace(a.Institution),
			Department:      strings.TrimSpace(a.Department),
			Designation:     strings.TrimSpace(a.Designation),
			AuthorOrder:     order,
			IsCorresponding: a.IsCorresponding || idx == 0,
		}
		if authorRec.Institution == "" {
			authorRec.Institution = institution
		}
		if authorRec.Email == "" {
			authorRec.Email = correspondingEmail
		}
		_ = tx.Create(&authorRec).Error
	}

	paperFile := models.PaperFile{
		PaperID:        paper.ID,
		FileType:       "INITIAL_SUBMISSION",
		OriginalName:   fileHeader.Filename,
		StoredFileName: storedName,
		StoragePath:    fullPath,
		MimeType:       "application/pdf",
		FileSize:       fileSize,
		VersionNumber:  1,
		UploadedByID:   studentID,
	}
	if err := tx.Create(&paperFile).Error; err != nil {
		tx.Rollback()
		utils.RespondError(c, http.StatusInternalServerError, "Failed to record uploaded PDF metadata", err.Error())
		return
	}

	remarks := "Research paper submitted by author."
	if initialStatus == models.StatusDraft {
		remarks = "Research paper saved as draft."
	}
	// Invalidate any stale formatting approval on non-published publication records
	_ = config.DB.Model(&models.Publication{}).
		Where("paper_id = ? AND is_published = ?", paper.ID, false).
		Updates(map[string]interface{}{
			"formatting_status":      models.FormattingNotStarted,
			"formatted_file_path":    "",
			"formatting_approved_at": nil,
			"formatting_approved_by": "",
		}).Error

	_ = repositories.RecordPaperStatusChange(tx, paper.ID, "", initialStatus, studentID, studentName, models.RoleStudent, remarks)

	tx.Commit()

	// Notify Student (in-app + email) and Super Admin
	services.CreateNotification(
		studentID,
		"Paper Submitted Successfully ("+paper.SubmissionCode+")",
		fmt.Sprintf("Your research paper '%s' has been submitted with Submission ID %s. Please wait while our Super Admin verifies and reviews your research paper.", paper.Title, paper.SubmissionCode),
		"SUBMISSION",
		paper.ID,
	)

	var admins []models.User
	config.DB.Where("role_name = ?", models.RoleSuperAdmin).Find(&admins)
	for _, ed := range admins {
		services.CreateNotification(
			ed.ID,
			"New Manuscript Submission: "+paper.SubmissionCode,
			fmt.Sprintf("%s submitted a new research paper: '%s'.", studentName, paper.Title),
			"SUBMISSION",
			paper.ID,
		)
	}

	services.LogAudit(studentID, studentEmail, models.RoleStudent, "PAPER_SUBMITTED", "research_papers", paper.ID, "Submitted paper "+paper.SubmissionCode, c.ClientIP())

	var fullPaper models.ResearchPaper
	repositories.PreloadPaperFull(config.DB).Where("id = ?", paper.ID).First(&fullPaper)
	sanitizePaperForStudent(&fullPaper)

	utils.RespondSuccess(c, http.StatusCreated, "Research paper submitted successfully", fullPaper)
}

// ListStudentPapers lists all submissions belonging to the logged-in student
func ListStudentPapers(c *gin.Context) {
	studentID := c.GetString("user_id")
	status := strings.TrimSpace(c.Query("status"))
	search := strings.TrimSpace(c.Query("search"))

	query := repositories.PreloadPaperFull(config.DB).Where("student_id = ?", studentID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(title) LIKE ? OR LOWER(submission_code) LIKE ? OR LOWER(keywords) LIKE ?", like, like, like)
	}

	var papers []models.ResearchPaper
	query.Order("created_at DESC").Find(&papers)
	for i := range papers {
		sanitizePaperForStudent(&papers[i])
	}

	utils.RespondSuccess(c, http.StatusOK, "Student research papers retrieved", papers)
}

// GetStudentPaperDetail retrieves a single research paper owned by the logged-in student
func GetStudentPaperDetail(c *gin.Context) {
	studentID := c.GetString("user_id")
	paperID := c.Param("id")

	var paper models.ResearchPaper
	if err := repositories.PreloadPaperFull(config.DB).
		Where("id = ? AND student_id = ?", paperID, studentID).
		First(&paper).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Research paper not found or access denied", nil)
		return
	}

	sanitizePaperForStudent(&paper)
	utils.RespondSuccess(c, http.StatusOK, "Research paper details retrieved", paper)
}

// SubmitPaperRevision allows a student to upload a revised PDF and response when revisions are requested (or when submitting a draft)
func SubmitPaperRevision(c *gin.Context) {
	studentID := c.GetString("user_id")
	studentName := c.GetString("user_name")
	studentEmail := c.GetString("user_email")
	paperID := c.Param("id")

	var paper models.ResearchPaper
	if err := config.DB.Preload("Files").Where("id = ? AND student_id = ?", paperID, studentID).First(&paper).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Research paper not found or access denied", nil)
		return
	}

	// Enforce workflow rule: Student can only edit and resubmit when revisions are requested or when paper is in DRAFT
	if paper.Status != models.StatusRevisionRequired && paper.Status != models.StatusDraft {
		utils.RespondError(c, http.StatusForbidden, fmt.Sprintf("Revisions can only be uploaded when paper status is REVISION_REQUIRED or DRAFT (current status: %s)", paper.Status), nil)
		return
	}

	authorResponse := strings.TrimSpace(c.PostForm("author_response"))
	changesSummary := strings.TrimSpace(c.PostForm("changes_summary"))
	updatedTitle := strings.TrimSpace(c.PostForm("title"))
	updatedAbstract := strings.TrimSpace(c.PostForm("abstract"))
	updatedKeywords := strings.TrimSpace(c.PostForm("keywords"))

	if paper.Status == models.StatusRevisionRequired && (authorResponse == "" || changesSummary == "") {
		utils.RespondError(c, http.StatusBadRequest, "Please provide both your response to reviewers and a summary of manuscript changes", nil)
		return
	}

	fileHeader, err := c.FormFile("pdf_file")
	if err != nil || fileHeader == nil {
		utils.RespondError(c, http.StatusBadRequest, "Please upload the revised Research Paper PDF file", nil)
		return
	}

	storedName, fullPath, fileSize, err := services.Storage.SavePDFFile(fileHeader, "revisions")
	if err != nil {
		utils.RespondError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	nextVersion := len(paper.Files) + 1
	oldStatus := paper.Status
	newStatus := models.StatusRevisionSubmitted
	nextRound := paper.CurrentRound + 1
	if oldStatus == models.StatusDraft {
		newStatus = models.StatusSubmitted
		nextRound = 1
	}

	tx := config.DB.Begin()

	paperFile := models.PaperFile{
		PaperID:        paper.ID,
		FileType:       "REVISION",
		OriginalName:   fileHeader.Filename,
		StoredFileName: storedName,
		StoragePath:    fullPath,
		MimeType:       "application/pdf",
		FileSize:       fileSize,
		VersionNumber:  nextVersion,
		UploadedByID:   studentID,
	}
	if err := tx.Create(&paperFile).Error; err != nil {
		tx.Rollback()
		utils.RespondError(c, http.StatusInternalServerError, "Failed to save revised PDF file record", err.Error())
		return
	}

	updates := map[string]interface{}{
		"status":        newStatus,
		"current_round": nextRound,
	}
	if updatedTitle != "" {
		updates["title"] = updatedTitle
	}
	if updatedAbstract != "" {
		updates["abstract"] = updatedAbstract
	}
	if updatedKeywords != "" {
		updates["keywords"] = updatedKeywords
	}
	_ = tx.Model(&paper).Updates(updates).Error

	// Update pending revision_history record or create one
	now := time.Now()
	var revHist models.RevisionHistory
	if err := tx.Where("paper_id = ? AND status = ?", paper.ID, "PENDING").Order("created_at DESC").First(&revHist).Error; err == nil {
		_ = tx.Model(&revHist).Updates(map[string]interface{}{
			"author_response": authorResponse,
			"changes_summary": changesSummary,
			"revised_file_id": paperFile.ID,
			"submitted_at":    now,
			"status":          "SUBMITTED",
		}).Error
	} else {
		_ = tx.Create(&models.RevisionHistory{
			PaperID:        paper.ID,
			RevisionRound:  paper.CurrentRound,
			AuthorResponse: authorResponse,
			ChangesSummary: changesSummary,
			RevisedFileID:  paperFile.ID,
			RequestedAt:    now,
			SubmittedAt:    &now,
			Status:         "SUBMITTED",
		}).Error
	}

	remarks := fmt.Sprintf("Revised manuscript (v%d) uploaded. Summary: %s", nextVersion, changesSummary)
	// Invalidate any stale formatting approval on non-published publication records
	_ = config.DB.Model(&models.Publication{}).
		Where("paper_id = ? AND is_published = ?", paper.ID, false).
		Updates(map[string]interface{}{
			"formatting_status":      models.FormattingNotStarted,
			"formatted_file_path":    "",
			"formatting_approved_at": nil,
			"formatting_approved_by": "",
		}).Error

	_ = repositories.RecordPaperStatusChange(tx, paper.ID, oldStatus, newStatus, studentID, studentName, models.RoleStudent, remarks)
	tx.Commit()

	// Notify Super Admin
	var admins []models.User
	config.DB.Where("role_name = ?", models.RoleSuperAdmin).Find(&admins)
	for _, ed := range admins {
		services.CreateNotification(
			ed.ID,
			"Revised Manuscript Submitted: "+paper.SubmissionCode,
			fmt.Sprintf("Author %s uploaded a revised manuscript for '%s'.", studentName, paper.Title),
			"REVISION",
			paper.ID,
		)
	}

	services.LogAudit(studentID, studentEmail, models.RoleStudent, "PAPER_REVISION_SUBMITTED", "research_papers", paper.ID, "Uploaded revision v"+fmt.Sprint(nextVersion), c.ClientIP())

	var updatedPaper models.ResearchPaper
	repositories.PreloadPaperFull(config.DB).Where("id = ?", paper.ID).First(&updatedPaper)
	sanitizePaperForStudent(&updatedPaper)

	utils.RespondSuccess(c, http.StatusOK, "Revised research paper uploaded and submitted successfully", updatedPaper)
}

// GetStudentCertificates lists all publication certificates belonging to the student's papers
func GetStudentCertificates(c *gin.Context) {
	studentID := c.GetString("user_id")
	var certificates []models.Certificate
	config.DB.
		Preload("Publication").
		Preload("Paper").
		Where("student_id = ? OR paper_id IN (SELECT id FROM research_papers WHERE student_id = ?)", studentID, studentID).
		Order("issued_at DESC").
		Find(&certificates)

	utils.RespondSuccess(c, http.StatusOK, "Student certificates retrieved", certificates)
}

// DownloadCertificatePDF streams the generated PDF certificate to an authorized user or via public certificate download
func DownloadCertificatePDF(c *gin.Context) {
	id := c.Param("id")
	var cert models.Certificate
	if err := config.DB.Where("id = ? OR UPPER(certificate_code) = UPPER(?)", id, id).First(&cert).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Certificate not found", nil)
		return
	}

	reader, contentType, size, err := services.Storage.ReadFile(cert.PDFStoragePath)
	if err != nil {
		// Re-generate certificate PDF on the fly if missing from storage
		storageDir := "./storage"
		if config.Cfg != nil && config.Cfg.StorageLocalPath != "" {
			storageDir = config.Cfg.StorageLocalPath
		}
		genPath, gErr := utils.GenerateCertificatePDF(
			storageDir,
			cert.CertificateCode,
			"ResearchHub",
			cert.PublicationName,
			cert.AuthorName,
			"",
			cert.PaperTitle,
			cert.PublicationCode,
			"",
			cert.Volume,
			cert.Issue,
			cert.PublicationDate.Format("January 02, 2006"),
			cert.SignatoryName,
			cert.SignatoryTitle,
			cert.QRVerificationURL,
		)
		if gErr != nil {
			utils.RespondError(c, http.StatusInternalServerError, "Failed to generate certificate PDF", gErr.Error())
			return
		}
		if _, objKey, uErr := services.Storage.SaveGeneratedFile(genPath, "certificates", "application/pdf"); uErr == nil {
			if objKey != genPath {
				_ = os.Remove(genPath)
			}
			_ = config.DB.Model(&cert).Update("pdf_storage_path", objKey).Error
			reader, contentType, size, err = services.Storage.ReadFile(objKey)
		} else {
			err = uErr
		}
	}
	if err != nil || reader == nil {
		utils.RespondError(c, http.StatusNotFound, "Certificate PDF unavailable", nil)
		return
	}
	defer reader.Close()

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_Certificate.pdf\"", cert.CertificateCode))
	c.DataFromReader(http.StatusOK, size, contentType, reader, nil)
}

// DownloadPaperFile streams a submitted or revised paper PDF with strict role & ownership authorization
func DownloadPaperFile(c *gin.Context) {
	userID := c.GetString("user_id")
	userRole := c.GetString("user_role")
	paperID := c.Param("id")
	fileID := c.Param("fileId")

	var paper models.ResearchPaper
	if err := config.DB.Preload("Files").Where("id = ?", paperID).First(&paper).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Research paper not found", nil)
		return
	}

	// Enforce ownership check
	if userRole == models.RoleStudent && paper.StudentID != userID {
		utils.RespondError(c, http.StatusForbidden, "Access denied: you do not own this submission", nil)
		return
	}

	// If published and ?version=original is not explicitly requested, serve the approved branded PDF
	if paper.Status == models.StatusPublished && c.Query("version") != "original" {
		var pub models.Publication
		if err := config.DB.Where("paper_id = ? AND is_published = ? AND published_file_path <> ''", paper.ID, true).First(&pub).Error; err == nil {
			if reader, ct, size, rErr := services.Storage.ReadFile(pub.PublishedFilePath); rErr == nil {
				defer reader.Close()
				c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"ResearchHub-%s-Research-Paper.pdf\"", pub.PublicationCode))
				c.DataFromReader(http.StatusOK, size, ct, reader, nil)
				return
			}
		}
	}

	var targetFile *models.PaperFile
	for i := range paper.Files {
		if fileID == "latest" || paper.Files[i].ID == fileID {
			targetFile = &paper.Files[i]
			break
		}
	}
	if targetFile == nil && len(paper.Files) > 0 {
		targetFile = &paper.Files[0]
	}
	if targetFile == nil {
		utils.RespondError(c, http.StatusNotFound, "Paper PDF file record not found", nil)
		return
	}

	reader, contentType, size, err := services.Storage.ReadFile(targetFile.StoragePath)
	if err != nil {
		storageDir := "./storage"
		if config.Cfg != nil && config.Cfg.StorageLocalPath != "" {
			storageDir = config.Cfg.StorageLocalPath
		}
		genPath, _, gErr := utils.GenerateSamplePaperPDF(storageDir, paper.SubmissionCode, paper.Title, paper.CorrespondingAuthor, paper.Institution, paper.Abstract, paper.Keywords)
		if gErr != nil {
			utils.RespondError(c, http.StatusNotFound, "File not found on storage", nil)
			return
		}
		if _, objKey, uErr := services.Storage.SaveGeneratedFile(genPath, "papers", "application/pdf"); uErr == nil {
			if objKey != genPath {
				_ = os.Remove(genPath)
			}
			_ = config.DB.Model(targetFile).Update("storage_path", objKey).Error
			reader, contentType, size, err = services.Storage.ReadFile(objKey)
		} else {
			err = uErr
		}
	}
	if err != nil || reader == nil {
		utils.RespondError(c, http.StatusNotFound, "File not found on storage", nil)
		return
	}
	defer reader.Close()

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", targetFile.OriginalName))
	c.DataFromReader(http.StatusOK, size, contentType, reader, nil)
}

func sanitizePaperForStudent(paper *models.ResearchPaper) {
	for i := range paper.Reviews {
		paper.Reviews[i].ConfidentialToEditor = ""
	}
	for i := range paper.Assignments {
		if paper.Assignments[i].Review != nil {
			paper.Assignments[i].Review.ConfidentialToEditor = ""
		}
	}
}
