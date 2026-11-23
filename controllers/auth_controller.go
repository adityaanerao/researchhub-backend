package controllers

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"researchhub-backend/config"
	"researchhub-backend/models"
	"researchhub-backend/services"
	"researchhub-backend/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type RegisterRequest struct {
	FullName         string `json:"full_name" binding:"required,min=2"`
	Email            string `json:"email" binding:"required,email"`
	Password         string `json:"password" binding:"required,min=6"`
	Phone            string `json:"phone" binding:"required"`
	University       string `json:"university" binding:"required"`
	Institution      string `json:"institution" binding:"required"`
	Department       string `json:"department" binding:"required"`
	DegreeProgram    string `json:"degree_program" binding:"required"`
	EnrollmentNumber string `json:"enrollment_number" binding:"required"`
	AcademicYear     string `json:"academic_year" binding:"required"`
	GuideName        string `json:"guide_name" binding:"required"`
	GuideEmail       string `json:"guide_email"`
	ResearchInterest string `json:"research_interest" binding:"required"`
	OTP              string `json:"otp" binding:"required"`
	Role             string `json:"role"` // Checked to forbid non-student registration
}

func generate6DigitOTP() string {
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "482910"
	}
	return fmt.Sprintf("%06d", n.Int64()+100000)
}

// SendRegisterOTP sends a 6-digit email verification OTP before creating a student account
func SendRegisterOTP(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		FullName string `json:"full_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Please enter a valid email address", err.Error())
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	var existing models.User
	if err := config.DB.Where("email = ?", email).First(&existing).Error; err == nil {
		utils.RespondError(c, http.StatusConflict, "An account with this email address already exists", nil)
		return
	}

	otp := generate6DigitOTP()
	_ = config.DB.Create(&models.EmailOTP{
		Email:     email,
		OTP:       otp,
		Purpose:   "REGISTER",
		ExpiresAt: time.Now().Add(15 * time.Minute),
		Used:      false,
	}).Error

	name := strings.TrimSpace(req.FullName)
	if name == "" {
		name = "Researcher"
	}
	go services.SendEmailAsync(
		email,
		name,
		"Email Verification OTP - ResearchHub",
		fmt.Sprintf("Your 6-digit OTP to verify your email and complete your ResearchHub Student Registration is: <strong style='font-size:20px;color:#6d28d9;'>%s</strong><br/><br/>This OTP is valid for 15 minutes.", otp),
	)

	utils.RespondSuccess(c, http.StatusOK, "Verification OTP has been sent to "+email, nil)
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=6"`
}

// Register handles public STUDENT registration only
func Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Validation failed: please check required registration fields", err.Error())
		return
	}

	// Strictly prevent public registration as Reviewer, Editor, or Super Admin
	if req.Role != "" && strings.ToUpper(req.Role) != models.RoleStudent {
		utils.RespondError(c, http.StatusForbidden, "Public registration is restricted to STUDENT accounts only. Administrative and reviewer accounts must be created by Super Admin.", nil)
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	var existing models.User
	if err := config.DB.Where("email = ?", email).First(&existing).Error; err == nil {
		utils.RespondError(c, http.StatusConflict, "An account with this email address already exists", nil)
		return
	}

	// Verify 6-digit email OTP before creating account
	var otpRec models.EmailOTP
	if err := config.DB.Where("email = ? AND otp = ? AND purpose = ? AND used = ?", email, strings.TrimSpace(req.OTP), "REGISTER", false).
		Order("created_at DESC").First(&otpRec).Error; err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Invalid email verification OTP. Please request a valid OTP.", nil)
		return
	}
	if time.Now().After(otpRec.ExpiresAt) {
		utils.RespondError(c, http.StatusBadRequest, "OTP has expired. Please request a new OTP.", nil)
		return
	}
	_ = config.DB.Model(&otpRec).Update("used", true).Error

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "Failed to hash password", nil)
		return
	}

	var studentRole models.Role
	_ = config.DB.Where("name = ?", models.RoleStudent).First(&studentRole).Error

	now := time.Now()
	user := models.User{
		Email:        email,
		PasswordHash: string(hashedBytes),
		FullName:     strings.TrimSpace(req.FullName),
		RoleID:       studentRole.ID,
		RoleName:     models.RoleStudent,
		Phone:        req.Phone,
		Institution:  req.Institution,
		Department:   req.Department,
		Designation:  req.DegreeProgram,
		IsActive:     true,
		LastLoginAt:  &now,
	}

	tx := config.DB.Begin()
	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		utils.RespondError(c, http.StatusInternalServerError, "Failed to create user account", err.Error())
		return
	}

	profile := models.StudentProfile{
		UserID:           user.ID,
		EnrollmentNumber: req.EnrollmentNumber,
		DegreeProgram:    req.DegreeProgram,
		AcademicYear:     req.AcademicYear,
		University:       req.University,
		Institution:      req.Institution,
		Department:       req.Department,
		GuideName:        req.GuideName,
		GuideEmail:       req.GuideEmail,
		ResearchInterest: req.ResearchInterest,
	}
	if err := tx.Create(&profile).Error; err != nil {
		tx.Rollback()
		utils.RespondError(c, http.StatusInternalServerError, "Failed to create student profile", err.Error())
		return
	}
	tx.Commit()

	user.StudentProfile = &profile
	token, expiresAt, err := utils.GenerateJWT(user.ID, user.Email, user.FullName, user.RoleName)
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "Failed to generate access token", nil)
		return
	}

	services.CreateNotification(
		user.ID,
		"Welcome to ResearchHub",
		"Your student researcher account has been registered. You can now submit research manuscripts and track peer-review progress.",
		"SUBMISSION",
		user.ID,
	)
	go services.SendEmailAsync(
		user.Email,
		user.FullName,
		"Welcome to ResearchHub — Account Created Successfully",
		fmt.Sprintf("Welcome to <strong>ResearchHub</strong>!<br/><br/>Your student researcher account (%s) has been verified and activated. You can now submit your research papers, track their review status, and download your official publication certificates.", user.Email),
	)
	services.LogAudit(user.ID, user.Email, user.RoleName, "STUDENT_REGISTERED", "users", user.ID, "New student registered", c.ClientIP())

	utils.RespondSuccess(c, http.StatusCreated, "Student registration completed successfully", gin.H{
		"token":      token,
		"expires_at": expiresAt,
		"user":       user,
	})
}

// SendAdminLoginOTP sends a 6-digit OTP to an active SUPER_ADMIN email (e.g. aaaaditya82@gmail.com)
func SendAdminLoginOTP(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Valid admin email address is required", err.Error())
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	var user models.User
	if err := config.DB.Where("email = ? AND role_name = ? AND is_active = ?", email, models.RoleSuperAdmin, true).First(&user).Error; err != nil {
		utils.RespondError(c, http.StatusForbidden, "This email is not authorized for Admin OTP login", nil)
		return
	}

	otp := generate6DigitOTP()
	_ = config.DB.Create(&models.EmailOTP{
		Email:     email,
		OTP:       otp,
		Purpose:   "ADMIN_LOGIN",
		ExpiresAt: time.Now().Add(15 * time.Minute),
		Used:      false,
	}).Error

	go services.SendEmailAsync(
		email,
		user.FullName,
		"Admin Login OTP - ResearchHub",
		fmt.Sprintf("Your 6-digit OTP to log in to the ResearchHub Admin Dashboard is: <strong style='font-size:20px;color:#6d28d9;'>%s</strong><br/><br/>This OTP is valid for 15 minutes.", otp),
	)

	utils.RespondSuccess(c, http.StatusOK, "Admin login OTP has been sent to "+email, nil)
}

// VerifyAdminLoginOTP verifies the 6-digit OTP and logs the admin into the dashboard
func VerifyAdminLoginOTP(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		OTP   string `json:"otp" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Email and OTP are required", err.Error())
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	var user models.User
	if err := config.DB.Where("email = ? AND role_name = ? AND is_active = ?", email, models.RoleSuperAdmin, true).First(&user).Error; err != nil {
		utils.RespondError(c, http.StatusForbidden, "Admin account not found or inactive", nil)
		return
	}

	var otpRec models.EmailOTP
	if err := config.DB.Where("email = ? AND otp = ? AND purpose = ? AND used = ?", email, strings.TrimSpace(req.OTP), "ADMIN_LOGIN", false).
		Order("created_at DESC").First(&otpRec).Error; err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Invalid OTP. Please request a valid OTP.", nil)
		return
	}
	if time.Now().After(otpRec.ExpiresAt) {
		utils.RespondError(c, http.StatusBadRequest, "OTP has expired. Please request a new OTP.", nil)
		return
	}
	_ = config.DB.Model(&otpRec).Update("used", true).Error

	now := time.Now()
	_ = config.DB.Model(&user).Update("last_login_at", now).Error

	token, _, err := utils.GenerateJWT(user.ID, user.Email, user.RoleName, user.FullName)
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "Failed to generate authentication token", nil)
		return
	}

	services.LogAudit(user.ID, user.Email, user.RoleName, "ADMIN_OTP_LOGIN", "users", user.ID, "Admin logged in via Email OTP", c.ClientIP())

	utils.RespondSuccess(c, http.StatusOK, "Admin login successful", gin.H{
		"token": token,
		"user":  user,
	})
}

// Login authenticates a user and issues a JWT access token
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Please enter a valid email and password", err.Error())
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	var user models.User
	if err := config.DB.
		Preload("StudentProfile").
		Preload("ReviewerProfile").
		Preload("EditorProfile").
		Where("email = ?", email).
		First(&user).Error; err != nil {
		utils.RespondError(c, http.StatusUnauthorized, "Invalid email address or password", nil)
		return
	}

	if !user.IsActive {
		utils.RespondError(c, http.StatusForbidden, "Your account is deactivated. Please contact the platform administrator.", nil)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		utils.RespondError(c, http.StatusUnauthorized, "Invalid email address or password", nil)
		return
	}

	now := time.Now()
	_ = config.DB.Model(&user).Update("last_login_at", now).Error

	token, expiresAt, err := utils.GenerateJWT(user.ID, user.Email, user.FullName, user.RoleName)
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "Failed to generate access token", nil)
		return
	}

	services.LogAudit(user.ID, user.Email, user.RoleName, "USER_LOGIN", "users", user.ID, "User logged in", c.ClientIP())

	utils.RespondSuccess(c, http.StatusOK, "Login successful", gin.H{
		"token":      token,
		"expires_at": expiresAt,
		"user":       user,
	})
}

// Logout records user logout
func Logout(c *gin.Context) {
	userID := c.GetString("user_id")
	userEmail := c.GetString("user_email")
	userRole := c.GetString("user_role")
	if userID != "" {
		services.LogAudit(userID, userEmail, userRole, "USER_LOGOUT", "users", userID, "User logged out", c.ClientIP())
	}
	utils.RespondSuccess(c, http.StatusOK, "Logged out successfully", nil)
}

// GetMe returns the currently authenticated user profile
func GetMe(c *gin.Context) {
	userID := c.GetString("user_id")
	var user models.User
	if err := config.DB.
		Preload("StudentProfile").
		Preload("ReviewerProfile").
		Preload("EditorProfile").
		Where("id = ?", userID).
		First(&user).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "User profile not found", nil)
		return
	}
	utils.RespondSuccess(c, http.StatusOK, "Authenticated user profile retrieved", user)
}

// ForgotPassword generates a password reset token
func ForgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Valid email is required", err.Error())
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	var user models.User
	if err := config.DB.Where("email = ?", email).First(&user).Error; err != nil {
		// Return success message to prevent email enumeration
		utils.RespondSuccess(c, http.StatusOK, "If that email is registered, a password reset link and token have been generated.", nil)
		return
	}

	resetToken := generate6DigitOTP()
	expiry := time.Now().Add(15 * time.Minute)
	_ = config.DB.Model(&user).Updates(map[string]interface{}{
		"reset_token":        resetToken,
		"reset_token_expiry": expiry,
	}).Error

	go services.SendEmailAsync(
		user.Email,
		user.FullName,
		"Password Reset OTP - ResearchHub",
		fmt.Sprintf("Your 6-digit OTP to reset your ResearchHub account password is: <strong style='font-size:20px;color:#6d28d9;'>%s</strong><br/><br/>This OTP is valid for 15 minutes.", resetToken),
	)
	services.LogAudit(user.ID, user.Email, user.RoleName, "FORGOT_PASSWORD", "users", user.ID, "Password reset OTP issued", c.ClientIP())

	utils.RespondSuccess(c, http.StatusOK, "A 6-digit password reset OTP has been sent to your registered email address.", gin.H{"expires_at": expiry})
}

// ResetPassword validates the reset token and sets a new password
func ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Reset token and new password (min 6 characters) are required", err.Error())
		return
	}

	var user models.User
	if err := config.DB.Where("reset_token = ?", strings.TrimSpace(req.Token)).First(&user).Error; err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Invalid or expired password reset token", nil)
		return
	}

	if user.ResetTokenExpiry == nil || time.Now().After(*user.ResetTokenExpiry) {
		utils.RespondError(c, http.StatusBadRequest, "Password reset token has expired. Please request a new one.", nil)
		return
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "Failed to hash new password", nil)
		return
	}

	_ = config.DB.Model(&user).Updates(map[string]interface{}{
		"password_hash":      string(hashedBytes),
		"reset_token":        "",
		"reset_token_expiry": nil,
	}).Error

	services.LogAudit(user.ID, user.Email, user.RoleName, "PASSWORD_RESET", "users", user.ID, "Password reset via token", c.ClientIP())
	utils.RespondSuccess(c, http.StatusOK, "Your password has been reset successfully. You can now sign in.", nil)
}

// ChangePassword updates password for an authenticated user
func ChangePassword(c *gin.Context) {
	userID := c.GetString("user_id")
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Current password and new password (min 6 chars) are required", err.Error())
		return
	}

	var user models.User
	if err := config.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "User not found", nil)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Current password is incorrect", nil)
		return
	}

	hashedBytes, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	_ = config.DB.Model(&user).Update("password_hash", string(hashedBytes)).Error

	services.LogAudit(user.ID, user.Email, user.RoleName, "PASSWORD_CHANGED", "users", user.ID, "User changed password", c.ClientIP())
	utils.RespondSuccess(c, http.StatusOK, "Password updated successfully", nil)
}

// GetMyNotifications returns notifications for the logged-in user
func GetMyNotifications(c *gin.Context) {
	userID := c.GetString("user_id")
	var list []models.Notification
	config.DB.Where("user_id = ?", userID).Order("created_at DESC").Limit(50).Find(&list)

	var unreadCount int64
	config.DB.Model(&models.Notification{}).Where("user_id = ? AND is_read = ?", userID, false).Count(&unreadCount)

	utils.RespondSuccess(c, http.StatusOK, "Notifications retrieved", gin.H{
		"notifications": list,
		"unread_count":  unreadCount,
	})
}

// MarkNotificationRead marks a single notification as read
func MarkNotificationRead(c *gin.Context) {
	userID := c.GetString("user_id")
	id := c.Param("id")
	now := time.Now()
	config.DB.Model(&models.Notification{}).Where("id = ? AND user_id = ?", id, userID).Updates(map[string]interface{}{
		"is_read": true,
		"read_at": now,
	})
	utils.RespondSuccess(c, http.StatusOK, "Notification marked as read", nil)
}

// MarkAllNotificationsRead marks all notifications for the current user as read
func MarkAllNotificationsRead(c *gin.Context) {
	userID := c.GetString("user_id")
	now := time.Now()
	config.DB.Model(&models.Notification{}).Where("user_id = ? AND is_read = ?", userID, false).Updates(map[string]interface{}{
		"is_read": true,
		"read_at": now,
	})
	utils.RespondSuccess(c, http.StatusOK, "All notifications marked as read", nil)
}
