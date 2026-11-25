package routes

import (
	"net/http"
	"time"

	"researchhub-backend/controllers"
	"researchhub-backend/docs"
	"researchhub-backend/middleware"
	"researchhub-backend/models"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()
	r.Use(middleware.CORSMiddleware())

	// Health check & Swagger API Documentation
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "ResearchHub Backend API"})
	})
	r.GET("/swagger", docs.ServeSwaggerUI)
	r.GET("/swagger/index.html", docs.ServeSwaggerUI)

	api := r.Group("/api/v1")
	{
		api.GET("/docs", docs.ServeSwaggerUI)
		api.GET("/docs/openapi.json", docs.ServeOpenAPISpec)

		// 1. Public Website & Repository Routes
		pub := api.Group("/public")
		{
			pub.GET("/home", controllers.GetPublicHome)
			pub.GET("/settings", controllers.GetPublicSettings)
			pub.GET("/content/:pageKey", controllers.GetWebsiteContent)
			pub.GET("/categories", controllers.GetPublicCategories)
			pub.GET("/editorial-board", controllers.GetEditorialBoard)
			pub.POST("/contact", controllers.SubmitContactMessage)
			pub.GET("/certificates/verify/:certificateId", controllers.VerifyCertificatePublic)
			pub.Static("/logos", "./storage/logos")
		}

		// Public Publications Repository
		api.GET("/publications", controllers.ListPublications)
		api.GET("/publications/:id", controllers.GetPublicationDetail)
		api.GET("/publications/:id/download", controllers.DownloadPublishedPDF)

		// Certificate PDF Download (Authorized or direct verification download)
		api.GET("/certificates/:id/download", controllers.DownloadCertificatePDF)

		// 2. Authentication Module
		auth := api.Group("/auth")
		{
			auth.POST("/send-register-otp", middleware.AuthRateLimiter(30, time.Minute), controllers.SendRegisterOTP)
			auth.POST("/register", middleware.AuthRateLimiter(30, time.Minute), controllers.Register)
			auth.POST("/login", middleware.AuthRateLimiter(40, time.Minute), controllers.Login)
			auth.POST("/admin-send-otp", middleware.AuthRateLimiter(20, time.Minute), controllers.SendAdminLoginOTP)
			auth.POST("/admin-verify-otp", middleware.AuthRateLimiter(20, time.Minute), controllers.VerifyAdminLoginOTP)
			auth.POST("/forgot-password", middleware.AuthRateLimiter(20, time.Minute), controllers.ForgotPassword)
			auth.POST("/reset-password", middleware.AuthRateLimiter(20, time.Minute), controllers.ResetPassword)

			authProtected := auth.Group("")
			authProtected.Use(middleware.AuthRequired())
			{
				authProtected.POST("/logout", controllers.Logout)
				authProtected.GET("/me", controllers.GetMe)
				authProtected.PUT("/change-password", controllers.ChangePassword)
			}
		}

		// 3. Authenticated Shared Routes (Notifications & Secure Paper File Download)
		protected := api.Group("")
		protected.Use(middleware.AuthRequired())
		{
			protected.GET("/notifications", controllers.GetMyNotifications)
			protected.PUT("/notifications/read-all", controllers.MarkAllNotificationsRead)
			protected.PUT("/notifications/:id/read", controllers.MarkNotificationRead)
			protected.GET("/papers/:id/files/:fileId/download", controllers.DownloadPaperFile)
		}

		// 4. Student Module (Role: STUDENT or SUPER_ADMIN)
		student := api.Group("/student")
		student.Use(middleware.AuthRequired(), middleware.RequireRoles(models.RoleStudent, models.RoleSuperAdmin))
		{
			student.GET("/dashboard", controllers.GetStudentDashboard)
			student.GET("/profile", controllers.GetStudentProfile)
			student.PUT("/profile", controllers.UpdateStudentProfile)
			student.POST("/papers", controllers.SubmitResearchPaper)
			student.GET("/papers", controllers.ListStudentPapers)
			student.GET("/papers/:id", controllers.GetStudentPaperDetail)
			student.POST("/papers/:id/revisions", controllers.SubmitPaperRevision)
			student.GET("/certificates", controllers.GetStudentCertificates)
			student.POST("/payments/create-order", controllers.CreatePaymentOrder)
			student.POST("/payments/verify", controllers.VerifyPayment)
			student.GET("/payments/:receiptNo/bill", controllers.DownloadPaymentBill)
		}

		// 5. Super Admin Module (Role: SUPER_ADMIN)
		admin := api.Group("/admin")
		admin.Use(middleware.AuthRequired(), middleware.RequireRoles(models.RoleSuperAdmin))
		{
			pubMgmt := admin.Group("/publications")
			{
				pubMgmt.GET("", controllers.ListAdminPublications)
				pubMgmt.POST("", controllers.CreateOrPreparePublication)
				pubMgmt.POST("/:id/publish", controllers.PublishPreparedRecord)
				pubMgmt.GET("/:id/email/status", controllers.AdminGetEmailStatus)
				pubMgmt.POST("/:id/email/resend", controllers.AdminResendPublicationEmail)
			}

			admin.POST("/papers/:id/format", controllers.AdminFormatPaper)
			admin.GET("/papers/:id/format/status", controllers.AdminGetFormatStatus)
			admin.GET("/papers/:id/format/preview", controllers.AdminPreviewFormattedPDF)
			admin.GET("/papers/:id/format/original", controllers.AdminDownloadOriginalPDF)
			admin.GET("/papers/:id/format/download", controllers.AdminDownloadFormattedPDF)
			admin.POST("/papers/:id/format/regenerate", controllers.AdminFormatPaper)
			admin.POST("/papers/:id/format/approve", controllers.AdminApproveFormattedPDF)

			admin.GET("/dashboard", controllers.GetAdminDashboard)
			admin.GET("/users", controllers.ListAdminUsers)
			admin.POST("/users", controllers.CreateAdminUser)
			admin.PUT("/users/:id", controllers.UpdateAdminUser)
			admin.DELETE("/users/:id", controllers.DeleteAdminUser)
			admin.GET("/papers", controllers.ListAdminPapers)
			admin.POST("/papers/:id/screening", controllers.AdminPerformScreening)
			admin.POST("/papers/:id/request-revision", controllers.AdminRequestRevision)
			admin.POST("/papers/:id/decision", controllers.AdminMakeDecision)
			admin.GET("/reports", controllers.GenerateAdminReport)
			admin.GET("/categories", controllers.ListAdminCategories)
			admin.POST("/categories", controllers.CreateAdminCategory)
			admin.PUT("/categories/:id", controllers.UpdateAdminCategory)
			admin.DELETE("/categories/:id", controllers.DeleteAdminCategory)
			admin.GET("/certificates", controllers.ListAdminCertificates)
			admin.PUT("/certificates/:id/revoke", controllers.ToggleRevokeCertificate)
			admin.GET("/website-contents", controllers.ListAdminWebsiteContents)
			admin.PUT("/website-contents/:pageKey", controllers.UpdateAdminWebsiteContent)
			admin.GET("/settings", controllers.ListAdminSettings)
			admin.PUT("/settings", controllers.UpdateAdminSettings)
			admin.GET("/contacts", controllers.ListContactMessages)
			admin.PUT("/contacts/:id", controllers.UpdateContactMessage)
			admin.GET("/audit-logs", controllers.ListAuditLogs)
		}
	}

	return r
}
