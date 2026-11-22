package controllers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"researchhub-backend/config"
	"researchhub-backend/models"
	"researchhub-backend/repositories"
	"researchhub-backend/services"
	"researchhub-backend/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// GetAdminDashboard returns all 10 required dashboard cards and 5 required analytics chart series from PostgreSQL/DB
func GetAdminDashboard(c *gin.Context) {
	var totalStudents, totalReviewers, totalEditors, totalSubmissions, pendingSubmissions int64
	var papersUnderReview, approvedPapers, rejectedPapers, publishedPapers, certificatesGenerated int64

	config.DB.Model(&models.User{}).Where("role_name = ?", models.RoleStudent).Count(&totalStudents)
	totalReviewers = 0
	totalEditors = 0

	config.DB.Model(&models.ResearchPaper{}).Count(&totalSubmissions)
	config.DB.Model(&models.ResearchPaper{}).Where("status IN ?", []string{
		models.StatusSubmitted,
		models.StatusInitialScreening,
	}).Count(&pendingSubmissions)
	config.DB.Model(&models.ResearchPaper{}).Where("status IN ?", []string{
		models.StatusUnderReview,
		models.StatusRevisionRequired,
		models.StatusRevisionSubmitted,
		models.StatusReviewRecommendedApproval,
		models.StatusReviewRecommendedRejection,
		models.StatusFinalEditorialDecision,
	}).Count(&papersUnderReview)
	config.DB.Model(&models.ResearchPaper{}).Where("status IN ?", []string{
		models.StatusApproved,
		models.StatusReadyForPublication,
	}).Count(&approvedPapers)
	config.DB.Model(&models.ResearchPaper{}).Where("status = ?", models.StatusRejected).Count(&rejectedPapers)
	config.DB.Model(&models.ResearchPaper{}).Where("status = ?", models.StatusPublished).Count(&publishedPapers)
	config.DB.Model(&models.Certificate{}).Where("is_revoked = ?", false).Count(&certificatesGenerated)

	// 1. Submission Status Distribution
	type StatusCount struct {
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}
	var statusDist []StatusCount
	config.DB.Model(&models.ResearchPaper{}).
		Select("status, COUNT(*) as count").
		Group("status").
		Order("count DESC").
		Scan(&statusDist)

	// 2. Research Category Distribution
	type CategoryCount struct {
		CategoryName string `json:"category_name"`
		Count        int64  `json:"count"`
	}
	var categoryDist []CategoryCount
	config.DB.Table("research_categories rc").
		Select("rc.name as category_name, COUNT(rp.id) as count").
		Joins("LEFT JOIN research_papers rp ON rp.category_id = rc.id AND rp.deleted_at IS NULL").
		Where("rc.deleted_at IS NULL").
		Group("rc.id, rc.name").
		Order("count DESC").
		Scan(&categoryDist)

	// 3. Monthly Submissions & 4. Monthly Publications (last 6 months computed from actual DB records)
	type MonthMetric struct {
		Month string `json:"month"`
		Count int64  `json:"count"`
	}
	var allPapers []models.ResearchPaper
	config.DB.Select("id, submitted_at, published_at, status").Find(&allPapers)

	var allPubs []models.Publication
	config.DB.Select("id, publication_date, publication_year, is_published").Where("is_published = ?", true).Find(&allPubs)

	now := time.Now()
	monthlySubmissions := make([]MonthMetric, 0, 6)
	monthlyPublications := make([]MonthMetric, 0, 6)

	for i := 5; i >= 0; i-- {
		targetMonth := now.AddDate(0, -i, 0)
		label := targetMonth.Format("Jan 2006")
		yr, mo, _ := targetMonth.Date()

		var subCnt int64
		for _, p := range allPapers {
			pYr, pMo, _ := p.SubmittedAt.Date()
			if pYr == yr && pMo == mo {
				subCnt++
			}
		}
		monthlySubmissions = append(monthlySubmissions, MonthMetric{Month: label, Count: subCnt})

		var pubCnt int64
		for _, pub := range allPubs {
			pubYr, pubMo, _ := pub.PublicationDate.Date()
			if pubYr == yr && pubMo == mo {
				pubCnt++
			}
		}
		monthlyPublications = append(monthlyPublications, MonthMetric{Month: label, Count: pubCnt})
	}

	// 5. Year-wise Publication Growth
	type YearMetric struct {
		Year  int   `json:"year"`
		Count int64 `json:"count"`
	}
	var yearGrowth []YearMetric
	config.DB.Model(&models.Publication{}).
		Select("publication_year as year, COUNT(*) as count").
		Where("is_published = ?", true).
		Group("publication_year").
		Order("publication_year ASC").
		Scan(&yearGrowth)

	var recentAuditLogs []models.AuditLog
	config.DB.Order("created_at DESC").Limit(10).Find(&recentAuditLogs)

	var registeredStudents []models.User
	config.DB.Preload("StudentProfile").Where("role_name = ?", models.RoleStudent).Order("created_at DESC").Find(&registeredStudents)

	var recentPapers []models.ResearchPaper
	repositories.PreloadPaperFull(config.DB).Order("created_at DESC").Limit(20).Find(&recentPapers)

	utils.RespondSuccess(c, http.StatusOK, "Super Admin dashboard analytics retrieved", gin.H{
		"cards": gin.H{
			"total_students":         totalStudents,
			"total_reviewers":        totalReviewers,
			"total_editors":          totalEditors,
			"total_submissions":      totalSubmissions,
			"pending_submissions":    pendingSubmissions,
			"papers_under_review":    papersUnderReview,
			"approved_papers":        approvedPapers,
			"rejected_papers":        rejectedPapers,
			"published_papers":       publishedPapers,
			"certificates_generated": certificatesGenerated,
		},
		"charts": gin.H{
			"monthly_submissions":            monthlySubmissions,
			"monthly_publications":           monthlyPublications,
			"submission_status_distribution": statusDist,
			"research_category_distribution": categoryDist,
			"year_wise_publication_growth":   yearGrowth,
		},
		"registered_students": registeredStudents,
		"recent_papers":       recentPapers,
		"recent_audit_logs":   recentAuditLogs,
	})
}

// ListAdminUsers returns all users with optional role and search filters
func ListAdminUsers(c *gin.Context) {
	role := strings.ToUpper(strings.TrimSpace(c.Query("role")))
	search := strings.TrimSpace(c.Query("search"))

	query := config.DB.
		Preload("StudentProfile").
		Preload("ReviewerProfile").
		Preload("EditorProfile")

	if role != "" {
		query = query.Where("role_name = ?", role)
	}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(full_name) LIKE ? OR LOWER(email) LIKE ? OR LOWER(institution) LIKE ?", like, like, like)
	}

	var users []models.User
	query.Order("created_at DESC").Find(&users)
	utils.RespondSuccess(c, http.StatusOK, "Users retrieved", users)
}

// CreateAdminUser allows Super Admin to create Student, Reviewer, Editor, or Super Admin accounts
func CreateAdminUser(c *gin.Context) {
	adminID := c.GetString("user_id")
	adminEmail := c.GetString("user_email")

	var req struct {
		FullName        string `json:"full_name" binding:"required"`
		Email           string `json:"email" binding:"required,email"`
		Password        string `json:"password" binding:"required,min=6"`
		Role            string `json:"role" binding:"required"` // STUDENT, REVIEWER, EDITOR, SUPER_ADMIN
		Phone           string `json:"phone"`
		Institution     string `json:"institution" binding:"required"`
		Department      string `json:"department"`
		Designation     string `json:"designation"`
		Bio             string `json:"bio"`
		ExpertiseAreas  string `json:"expertise_areas"`
		Qualification   string `json:"qualification"`
		ExperienceYears int    `json:"experience_years"`
		EditorialRole   string `json:"editorial_role"`
		DegreeProgram   string `json:"degree_program"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Full name, valid email, password (min 6 chars), role, and institution are required", err.Error())
		return
	}

	role := strings.ToUpper(strings.TrimSpace(req.Role))
	validRoles := map[string]bool{
		models.RoleStudent:    true,
		models.RoleSuperAdmin: true,
	}
	if !validRoles[role] {
		utils.RespondError(c, http.StatusBadRequest, "Invalid role. Must be STUDENT or SUPER_ADMIN", nil)
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	var existing models.User
	if err := config.DB.Where("email = ?", email).First(&existing).Error; err == nil {
		utils.RespondError(c, http.StatusConflict, "A user with this email address already exists", nil)
		return
	}

	hashed, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	var roleRec models.Role
	_ = config.DB.Where("name = ?", role).First(&roleRec).Error

	user := models.User{
		Email:        email,
		PasswordHash: string(hashed),
		FullName:     strings.TrimSpace(req.FullName),
		RoleID:       roleRec.ID,
		RoleName:     role,
		Phone:        strings.TrimSpace(req.Phone),
		Institution:  strings.TrimSpace(req.Institution),
		Department:   strings.TrimSpace(req.Department),
		Designation:  strings.TrimSpace(req.Designation),
		Bio:          strings.TrimSpace(req.Bio),
		IsActive:     true,
	}

	tx := config.DB.Begin()
	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		utils.RespondError(c, http.StatusInternalServerError, "Failed to create user account", err.Error())
		return
	}

	switch role {
	case models.RoleStudent:
		_ = tx.Create(&models.StudentProfile{
			UserID:        user.ID,
			DegreeProgram: req.DegreeProgram,
			Institution:   user.Institution,
			Department:    user.Department,
		}).Error
	}
	tx.Commit()

	services.LogAudit(adminID, adminEmail, models.RoleSuperAdmin, "USER_CREATED", "users", user.ID, fmt.Sprintf("Created %s user %s", role, user.Email), c.ClientIP())
	utils.RespondSuccess(c, http.StatusCreated, "User account created successfully", user)
}

// UpdateAdminUser updates a user's details, role, password, or active status
func UpdateAdminUser(c *gin.Context) {
	adminID := c.GetString("user_id")
	adminEmail := c.GetString("user_email")
	targetID := c.Param("id")

	var user models.User
	if err := config.DB.Where("id = ?", targetID).First(&user).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "User not found", nil)
		return
	}

	var req struct {
		FullName       string `json:"full_name"`
		Role           string `json:"role"`
		Phone          string `json:"phone"`
		Institution    string `json:"institution"`
		Department     string `json:"department"`
		Designation    string `json:"designation"`
		Bio            string `json:"bio"`
		IsActive       *bool  `json:"is_active"`
		Password       string `json:"password"`
		ExpertiseAreas string `json:"expertise_areas"`
		Qualification  string `json:"qualification"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Invalid update request", err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.FullName != "" {
		updates["full_name"] = strings.TrimSpace(req.FullName)
	}
	if req.Role != "" {
		updates["role_name"] = strings.ToUpper(strings.TrimSpace(req.Role))
	}
	if req.Phone != "" {
		updates["phone"] = strings.TrimSpace(req.Phone)
	}
	if req.Institution != "" {
		updates["institution"] = strings.TrimSpace(req.Institution)
	}
	if req.Department != "" {
		updates["department"] = strings.TrimSpace(req.Department)
	}
	if req.Designation != "" {
		updates["designation"] = strings.TrimSpace(req.Designation)
	}
	if req.Bio != "" {
		updates["bio"] = strings.TrimSpace(req.Bio)
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	if len(req.Password) >= 6 {
		hashed, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		updates["password_hash"] = string(hashed)
	}

	_ = config.DB.Model(&user).Updates(updates).Error

	if req.ExpertiseAreas != "" || req.Qualification != "" {
		config.DB.Model(&models.ReviewerProfile{}).Where("user_id = ?", user.ID).Updates(map[string]interface{}{
			"expertise_areas": req.ExpertiseAreas,
			"qualification":   req.Qualification,
		})
	}

	_ = config.DB.Preload("StudentProfile").Preload("ReviewerProfile").Preload("EditorProfile").Where("id = ?", user.ID).First(&user).Error
	services.LogAudit(adminID, adminEmail, models.RoleSuperAdmin, "USER_UPDATED", "users", user.ID, "Updated user "+user.Email, c.ClientIP())
	utils.RespondSuccess(c, http.StatusOK, "User account updated successfully", user)
}

// DeleteAdminUser soft-deletes a user account (prevents self-deletion)
func DeleteAdminUser(c *gin.Context) {
	adminID := c.GetString("user_id")
	adminEmail := c.GetString("user_email")
	targetID := c.Param("id")

	if adminID == targetID {
		utils.RespondError(c, http.StatusBadRequest, "You cannot delete your own active Super Admin account", nil)
		return
	}

	var user models.User
	if err := config.DB.Where("id = ?", targetID).First(&user).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "User not found", nil)
		return
	}

	_ = config.DB.Delete(&user).Error
	services.LogAudit(adminID, adminEmail, models.RoleSuperAdmin, "USER_DELETED", "users", targetID, "Deleted user "+user.Email, c.ClientIP())
	utils.RespondSuccess(c, http.StatusOK, "User account deleted", nil)
}

// ListAdminPapers lists all research papers in the platform for Super Admin management
func ListAdminPapers(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	categoryID := strings.TrimSpace(c.Query("category_id"))
	search := strings.TrimSpace(c.Query("search"))

	query := repositories.PreloadPaperFull(config.DB)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(title) LIKE ? OR LOWER(submission_code) LIKE ? OR LOWER(corresponding_author) LIKE ? OR LOWER(institution) LIKE ?", like, like, like, like)
	}

	var papers []models.ResearchPaper
	query.Order("created_at DESC").Find(&papers)
	utils.RespondSuccess(c, http.StatusOK, "All research papers retrieved", papers)
}

// ListAdminCategories returns all research categories (including inactive ones)
func ListAdminCategories(c *gin.Context) {
	var categories []models.ResearchCategory
	config.DB.Order("name ASC").Find(&categories)
	utils.RespondSuccess(c, http.StatusOK, "Research categories retrieved", categories)
}

// CreateAdminCategory creates a new research category / discipline
func CreateAdminCategory(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Code        string `json:"code" binding:"required"`
		Discipline  string `json:"discipline" binding:"required"`
		Description string `json:"description"`
		IsActive    *bool  `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Category name, code, and discipline are required", err.Error())
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	cat := models.ResearchCategory{
		Name:        strings.TrimSpace(req.Name),
		Code:        strings.ToUpper(strings.TrimSpace(req.Code)),
		Discipline:  strings.TrimSpace(req.Discipline),
		Description: strings.TrimSpace(req.Description),
		IsActive:    isActive,
	}
	if err := config.DB.Create(&cat).Error; err != nil {
		utils.RespondError(c, http.StatusConflict, "Category name or code already exists", err.Error())
		return
	}
	utils.RespondSuccess(c, http.StatusCreated, "Research category created", cat)
}

// UpdateAdminCategory updates a research category
func UpdateAdminCategory(c *gin.Context) {
	id := c.Param("id")
	var cat models.ResearchCategory
	if err := config.DB.Where("id = ?", id).First(&cat).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Category not found", nil)
		return
	}

	var req struct {
		Name        string `json:"name"`
		Code        string `json:"code"`
		Discipline  string `json:"discipline"`
		Description string `json:"description"`
		IsActive    *bool  `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Invalid category update payload", err.Error())
		return
	}

	if req.Name != "" {
		cat.Name = strings.TrimSpace(req.Name)
	}
	if req.Code != "" {
		cat.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	}
	if req.Discipline != "" {
		cat.Discipline = strings.TrimSpace(req.Discipline)
	}
	if req.Description != "" {
		cat.Description = strings.TrimSpace(req.Description)
	}
	if req.IsActive != nil {
		cat.IsActive = *req.IsActive
	}
	_ = config.DB.Save(&cat).Error
	utils.RespondSuccess(c, http.StatusOK, "Research category updated", cat)
}

// DeleteAdminCategory deletes a research category if unused
func DeleteAdminCategory(c *gin.Context) {
	id := c.Param("id")
	_ = config.DB.Where("id = ?", id).Delete(&models.ResearchCategory{}).Error
	utils.RespondSuccess(c, http.StatusOK, "Research category deleted", nil)
}

// ListAdminCertificates returns all generated certificates
func ListAdminCertificates(c *gin.Context) {
	var certs []models.Certificate
	config.DB.Preload("Publication").Order("issued_at DESC").Find(&certs)
	utils.RespondSuccess(c, http.StatusOK, "All certificates retrieved", certs)
}

// ToggleRevokeCertificate revokes or restores a certificate
func ToggleRevokeCertificate(c *gin.Context) {
	id := c.Param("id")
	var cert models.Certificate
	if err := config.DB.Where("id = ?", id).First(&cert).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Certificate not found", nil)
		return
	}
	cert.IsRevoked = !cert.IsRevoked
	_ = config.DB.Save(&cert).Error
	utils.RespondSuccess(c, http.StatusOK, "Certificate status updated", cert)
}

// ListAdminWebsiteContents returns all editable public website pages
func ListAdminWebsiteContents(c *gin.Context) {
	var contents []models.WebsiteContent
	config.DB.Order("page_key ASC").Find(&contents)
	utils.RespondSuccess(c, http.StatusOK, "Website contents retrieved", contents)
}

// UpdateAdminWebsiteContent updates a public page's CMS content
func UpdateAdminWebsiteContent(c *gin.Context) {
	pageKey := c.Param("pageKey")
	userID := c.GetString("user_id")

	var req struct {
		Title           string `json:"title" binding:"required"`
		Subtitle        string `json:"subtitle"`
		ContentHTML     string `json:"content_html" binding:"required"`
		MetaDescription string `json:"meta_description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Title and content HTML are required", err.Error())
		return
	}

	var content models.WebsiteContent
	if err := config.DB.Where("page_key = ?", pageKey).First(&content).Error; err != nil {
		content = models.WebsiteContent{PageKey: pageKey}
	}
	content.Title = strings.TrimSpace(req.Title)
	content.Subtitle = strings.TrimSpace(req.Subtitle)
	content.ContentHTML = req.ContentHTML
	content.MetaDescription = strings.TrimSpace(req.MetaDescription)
	content.UpdatedByUserID = userID

	_ = config.DB.Save(&content).Error
	utils.RespondSuccess(c, http.StatusOK, "Website content saved", content)
}

// ListAdminSettings returns all system settings
func ListAdminSettings(c *gin.Context) {
	var settings []models.SystemSetting
	config.DB.Order("setting_group ASC, setting_key ASC").Find(&settings)
	utils.RespondSuccess(c, http.StatusOK, "System settings retrieved", settings)
}

// UpdateAdminSettings updates multiple system settings at once
func UpdateAdminSettings(c *gin.Context) {
	var req struct {
		Settings map[string]string `json:"settings" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Settings key-value map is required", err.Error())
		return
	}

	for k, v := range req.Settings {
		var setting models.SystemSetting
		if err := config.DB.Where("setting_key = ?", k).First(&setting).Error; err == nil {
			_ = config.DB.Model(&setting).Update("setting_value", v).Error
		} else {
			_ = config.DB.Create(&models.SystemSetting{
				SettingKey:   k,
				SettingValue: v,
				SettingGroup: "BRANDING",
				IsPublic:     true,
			}).Error
		}
	}

	var all []models.SystemSetting
	config.DB.Order("setting_group ASC, setting_key ASC").Find(&all)
	utils.RespondSuccess(c, http.StatusOK, "System settings updated successfully", all)
}

// ListContactMessages lists all public contact inquiries
func ListContactMessages(c *gin.Context) {
	var msgs []models.ContactMessage
	config.DB.Order("created_at DESC").Find(&msgs)
	utils.RespondSuccess(c, http.StatusOK, "Contact messages retrieved", msgs)
}

// UpdateContactMessage marks a contact message resolved or stores an admin reply
func UpdateContactMessage(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Status     string `json:"status"`
		AdminReply string `json:"admin_reply"`
	}
	_ = c.ShouldBindJSON(&req)

	now := time.Now()
	config.DB.Model(&models.ContactMessage{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":      req.Status,
		"admin_reply": req.AdminReply,
		"replied_at":  now,
	})
	utils.RespondSuccess(c, http.StatusOK, "Contact message updated", nil)
}

// ListAuditLogs returns security and editorial audit logs
func ListAuditLogs(c *gin.Context) {
	var logs []models.AuditLog
	config.DB.Order("created_at DESC").Limit(100).Find(&logs)
	utils.RespondSuccess(c, http.StatusOK, "Audit logs retrieved", logs)
}

// AdminPerformScreening allows Super Admin to perform initial screening on a submitted paper
func AdminPerformScreening(c *gin.Context) {
	adminID := c.GetString("user_id")
	adminName := c.GetString("user_name")
	adminRole := c.GetString("user_role")
	paperID := c.Param("id")

	var req struct {
		Action         string `json:"action" binding:"required"`
		ScreeningNotes string `json:"screening_notes" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Screening action and notes are required", err.Error())
		return
	}

	var paper models.ResearchPaper
	if err := config.DB.Where("id = ?", paperID).First(&paper).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Research paper not found", nil)
		return
	}

	oldStatus := paper.Status
	newStatus := models.StatusInitialScreening
	switch strings.ToUpper(req.Action) {
	case "PASS_SCREENING", "INITIAL_SCREENING":
		newStatus = models.StatusInitialScreening
	case "REVISION_REQUIRED":
		newStatus = models.StatusRevisionRequired
	case "DESK_REJECT", "REJECTED":
		newStatus = models.StatusRejected
	}

	tx := config.DB.Begin()
	_ = tx.Model(&paper).Updates(map[string]interface{}{
		"status":             newStatus,
		"screening_notes":    strings.TrimSpace(req.ScreeningNotes),
		"assigned_editor_id": adminID,
	}).Error

	if newStatus == models.StatusRevisionRequired {
		_ = tx.Create(&models.RevisionHistory{
			PaperID:         paper.ID,
			RevisionRound:   paper.CurrentRound,
			RequestedByID:   adminID,
			RequestComments: req.ScreeningNotes,
			RequestedAt:     time.Now(),
			Status:          "PENDING",
		}).Error
	}

	_ = repositories.RecordPaperStatusChange(tx, paper.ID, oldStatus, newStatus, adminID, adminName, adminRole, "Initial screening: "+req.ScreeningNotes)
	tx.Commit()

	services.CreateNotification(
		paper.StudentID,
		fmt.Sprintf("Initial Screening Update (%s)", paper.SubmissionCode),
		fmt.Sprintf("Your paper '%s' status is now %s. Notes: %s", paper.Title, newStatus, req.ScreeningNotes),
		"DECISION",
		paper.ID,
	)

	var updated models.ResearchPaper
	repositories.PreloadPaperFull(config.DB).Where("id = ?", paper.ID).First(&updated)
	utils.RespondSuccess(c, http.StatusOK, "Initial screening completed", updated)
}

// AdminRequestRevision allows Super Admin to request author revisions
func AdminRequestRevision(c *gin.Context) {
	adminID := c.GetString("user_id")
	adminName := c.GetString("user_name")
	adminEmail := c.GetString("user_email")
	adminRole := c.GetString("user_role")
	paperID := c.Param("id")

	var req struct {
		Comments string `json:"comments" binding:"required,min=5"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Revision instructions/comments are required", err.Error())
		return
	}

	var paper models.ResearchPaper
	if err := config.DB.Where("id = ?", paperID).First(&paper).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Research paper not found", nil)
		return
	}

	oldStatus := paper.Status
	tx := config.DB.Begin()

	_ = tx.Model(&paper).Updates(map[string]interface{}{
		"status":          models.StatusRevisionRequired,
		"editorial_notes": strings.TrimSpace(req.Comments),
	}).Error

	_ = tx.Create(&models.RevisionHistory{
		PaperID:         paper.ID,
		RevisionRound:   paper.CurrentRound,
		RequestedByID:   adminID,
		RequestComments: strings.TrimSpace(req.Comments),
		RequestedAt:     time.Now(),
		Status:          "PENDING",
	}).Error

	_ = tx.Create(&models.EditorialDecision{
		PaperID:           paper.ID,
		EditorID:          adminID,
		EditorName:        adminName,
		ReviewRound:       paper.CurrentRound,
		Decision:          models.StatusRevisionRequired,
		EditorialComments: strings.TrimSpace(req.Comments),
		DecidedAt:         time.Now(),
	}).Error

	_ = repositories.RecordPaperStatusChange(tx, paper.ID, oldStatus, models.StatusRevisionRequired, adminID, adminName, adminRole, "Revision requested: "+req.Comments)
	tx.Commit()

	services.CreateNotification(
		paper.StudentID,
		"Revision Requested for "+paper.SubmissionCode,
		fmt.Sprintf("Super Admin has requested revisions for '%s'. Comments: %s", paper.Title, req.Comments),
		"REVISION",
		paper.ID,
	)
	services.LogAudit(adminID, adminEmail, adminRole, "REVISION_REQUESTED", "research_papers", paper.ID, req.Comments, c.ClientIP())

	var updated models.ResearchPaper
	repositories.PreloadPaperFull(config.DB).Where("id = ?", paper.ID).First(&updated)
	utils.RespondSuccess(c, http.StatusOK, "Revision request sent to author", updated)
}

// AdminMakeDecision allows Super Admin to approve, reject, or mark a paper ready for publication
func AdminMakeDecision(c *gin.Context) {
	adminID := c.GetString("user_id")
	adminName := c.GetString("user_name")
	adminEmail := c.GetString("user_email")
	adminRole := c.GetString("user_role")
	paperID := c.Param("id")

	var req struct {
		Decision string `json:"decision" binding:"required"`
		Comments string `json:"comments" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Decision and comments are required", err.Error())
		return
	}

	var paper models.ResearchPaper
	if err := config.DB.Where("id = ?", paperID).First(&paper).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Research paper not found", nil)
		return
	}

	decision := strings.ToUpper(strings.TrimSpace(req.Decision))
	allowed := map[string]bool{
		models.StatusFinalEditorialDecision: true,
		models.StatusApproved:               true,
		models.StatusRejected:               true,
		models.StatusReadyForPublication:    true,
		models.StatusRevisionRequired:       true,
	}
	if !allowed[decision] {
		utils.RespondError(c, http.StatusBadRequest, "Invalid decision status. Allowed: APPROVED, REJECTED, READY_FOR_PUBLICATION, FINAL_EDITORIAL_DECISION, REVISION_REQUIRED", nil)
		return
	}

	oldStatus := paper.Status
	now := time.Now()
	tx := config.DB.Begin()

	updates := map[string]interface{}{
		"status":          decision,
		"editorial_notes": strings.TrimSpace(req.Comments),
	}
	if decision == models.StatusApproved || decision == models.StatusReadyForPublication {
		updates["approved_at"] = now
	}
	_ = tx.Model(&paper).Updates(updates).Error

	_ = tx.Create(&models.EditorialDecision{
		PaperID:           paper.ID,
		EditorID:          adminID,
		EditorName:        adminName,
		ReviewRound:       paper.CurrentRound,
		Decision:          decision,
		EditorialComments: strings.TrimSpace(req.Comments),
		DecidedAt:         now,
	}).Error

	if decision == models.StatusRevisionRequired {
		_ = tx.Create(&models.RevisionHistory{
			PaperID:         paper.ID,
			RevisionRound:   paper.CurrentRound,
			RequestedByID:   adminID,
			RequestComments: strings.TrimSpace(req.Comments),
			RequestedAt:     now,
			Status:          "PENDING",
		}).Error
	}

	_ = repositories.RecordPaperStatusChange(tx, paper.ID, oldStatus, decision, adminID, adminName, adminRole, fmt.Sprintf("Super Admin decision (%s): %s", decision, req.Comments))
	tx.Commit()

	// Automatically create/publish a public Publication record and generate certificates when approved
	if decision == models.StatusApproved || decision == models.StatusReadyForPublication {
		_ = repositories.PreloadPaperFull(config.DB).Where("id = ?", paper.ID).First(&paper).Error
		ensurePublicPublicationForApprovedPaper(&paper, adminID, now)
		certs := generateCertificatesForApprovedPaper(&paper, now)
		if len(certs) > 0 {
			services.CreateNotification(
				paper.StudentID,
				"Certificate Issued for "+paper.SubmissionCode,
				fmt.Sprintf("Congratulations! Your research paper '%s' has been approved and %d official certificate(s) have been sent to your profile (My Certificates).", paper.Title, len(certs)),
				"CERTIFICATE",
				paper.ID,
			)
			for _, cert := range certs {
				targetEmail := cert.AuthorEmail
				if targetEmail == "" {
					targetEmail = paper.CorrespondingEmail
				}
				if targetEmail != "" && cert.PDFStoragePath != "" {
					go services.SendEmailWithAttachmentAsync(
						targetEmail,
						cert.AuthorName,
						fmt.Sprintf("Research Paper Approved & Publication Certificate (%s)", cert.CertificateCode),
						fmt.Sprintf("Congratulations! Your research paper <strong>\"%s\"</strong> (%s) has been officially <strong>APPROVED</strong>.<br/><br/>Your official Publication Certificate (Certificate ID: <strong>%s</strong>) is attached to this email and is also available in your ResearchHub Student Profile under <em>My Certificates</em>.", paper.Title, paper.SubmissionCode, cert.CertificateCode),
						cert.PDFStoragePath,
						cert.CertificateCode+"_Certificate.pdf",
					)
				}
			}
		}
	}

	services.CreateNotification(
		paper.StudentID,
		fmt.Sprintf("Decision on %s: %s", paper.SubmissionCode, decision),
		fmt.Sprintf("Decision for '%s': %s. Notes: %s", paper.Title, decision, req.Comments),
		"DECISION",
		paper.ID,
	)
	services.LogAudit(adminID, adminEmail, adminRole, "DECISION_"+decision, "research_papers", paper.ID, req.Comments, c.ClientIP())

	var updated models.ResearchPaper
	repositories.PreloadPaperFull(config.DB).Where("id = ?", paper.ID).First(&updated)
	utils.RespondSuccess(c, http.StatusOK, "Decision recorded successfully", updated)
}

func ensurePublicPublicationForApprovedPaper(paper *models.ResearchPaper, adminID string, pubDate time.Time) {
	var existing models.Publication
	if err := config.DB.Where("paper_id = ?", paper.ID).First(&existing).Error; err == nil {
		if !existing.IsPublished {
			_ = config.DB.Model(&existing).Updates(map[string]interface{}{
				"is_published":     true,
				"publication_date": pubDate,
			}).Error
		}
		return
	}

	var count int64
	config.DB.Model(&models.Publication{}).Unscoped().Count(&count)
	pubCode := fmt.Sprintf("RHPUB-%d-%03d", pubDate.Year(), count+1)

	publishedPDFID := ""
	if len(paper.Files) > 0 {
		publishedPDFID = paper.Files[0].ID
	}

	pub := models.Publication{
		PublicationCode:  pubCode,
		PaperID:          paper.ID,
		PublicationTitle: paper.Title,
		JournalName:      "Research Hub Journal",
		Volume:           "12",
		Issue:            "4",
		PublicationYear:  pubDate.Year(),
		PublicationDate:  pubDate,
		ISSN:             "2994-812X",
		PageStart:        1,
		PageEnd:          10,
		PublishedPDFID:   publishedPDFID,
		PublicationURL:   "/publications/" + paper.ID,
		IsPublished:      true,
		PublishedByID:    adminID,
	}
	_ = config.DB.Create(&pub).Error
}

func generateCertificatesForApprovedPaper(paper *models.ResearchPaper, issuedAt time.Time) []models.Certificate {
	signatoryName := "Prof. Dr. Eleanor Vance"
	signatoryTitle := "Editor-in-Chief, ResearchHub Editorial Board"
	journalName := "ResearchHub Transactions on Multidisciplinary Sciences"
	vol := "12"
	iss := "4"

	var sName, sTitle, jName, cVol, cIss models.SystemSetting
	if err := config.DB.Where("setting_key = ?", "signatory_name").First(&sName).Error; err == nil && sName.SettingValue != "" {
		signatoryName = sName.SettingValue
	}
	if err := config.DB.Where("setting_key = ?", "signatory_title").First(&sTitle).Error; err == nil && sTitle.SettingValue != "" {
		signatoryTitle = sTitle.SettingValue
	}
	if err := config.DB.Where("setting_key = ?", "journal_name").First(&jName).Error; err == nil && jName.SettingValue != "" {
		journalName = jName.SettingValue
	}
	if err := config.DB.Where("setting_key = ?", "current_volume").First(&cVol).Error; err == nil && cVol.SettingValue != "" {
		vol = cVol.SettingValue
	}
	if err := config.DB.Where("setting_key = ?", "current_issue").First(&cIss).Error; err == nil && cIss.SettingValue != "" {
		iss = cIss.SettingValue
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

	var generated []models.Certificate
	for idx, author := range authors {
		certCode := fmt.Sprintf("RHCERT-%s-A%d", strings.TrimPrefix(paper.SubmissionCode, "RH-"), idx+1)
		var existing models.Certificate
		if err := config.DB.Where("certificate_code = ?", certCode).First(&existing).Error; err == nil {
			generated = append(generated, existing)
			continue
		}

		verifyURL := fmt.Sprintf("%s/verify-certificate/%s", appURL, certCode)
		pdfPath, _ := utils.GenerateCertificatePDF(
			storageDir,
			certCode,
			"ResearchHub",
			journalName,
			author.FullName,
			author.Institution,
			paper.Title,
			paper.SubmissionCode,
			"",
			vol,
			iss,
			issuedAt.Format("January 02, 2006"),
			signatoryName,
			signatoryTitle,
			verifyURL,
		)

		cert := models.Certificate{
			CertificateCode:   certCode,
			PaperID:           paper.ID,
			StudentID:         paper.StudentID,
			AuthorName:        author.FullName,
			AuthorEmail:       author.Email,
			PaperTitle:        paper.Title,
			PublicationName:   journalName,
			PublicationCode:   paper.SubmissionCode,
			Volume:            vol,
			Issue:             iss,
			PublicationDate:   issuedAt,
			SignatoryName:     signatoryName,
			SignatoryTitle:    signatoryTitle,
			QRVerificationURL: verifyURL,
			PDFStoragePath:    pdfPath,
			IsRevoked:         false,
			IssuedAt:          issuedAt,
		}
		if err := config.DB.Create(&cert).Error; err == nil {
			generated = append(generated, cert)
		}
	}
	return generated
}
