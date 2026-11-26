package migrations

import (
	"fmt"
	"log"

	"researchhub-backend/config"
	"researchhub-backend/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// RunMigrations executes GORM AutoMigrate on all 24 normalized tables
func RunMigrations(db *gorm.DB) error {
	err := db.AutoMigrate(
		&models.Role{},
		&models.User{},
		&models.StudentProfile{},
		&models.ReviewerProfile{},
		&models.EditorProfile{},
		&models.ResearchCategory{},
		&models.ResearchPaper{},
		&models.PaperAuthor{},
		&models.PaperFile{},
		&models.PaperStatusHistory{},
		&models.PaperAssignment{},
		&models.PaperReview{},
		&models.ReviewComment{},
		&models.RevisionHistory{},
		&models.EditorialDecision{},
		&models.PublicationIssue{},
		&models.Publication{},
		&models.Certificate{},
		&models.CertificateVerification{},
		&models.Notification{},
		&models.ContactMessage{},
		&models.WebsiteContent{},
		&models.SystemSetting{},
		&models.AuditLog{},
		&models.EmailOTP{},
		&models.Payment{},
		&models.FormattingJob{},
		&models.EmailJob{},
	)
	if err != nil {
		return fmt.Errorf("auto migration failed: %w", err)
	}
	log.Println("[INFO] All 24 normalized database tables migrated successfully.")
	return nil
}

// SeedDevelopmentData seeds system roles, settings, website contents, categories, and core staff accounts (without dummy students or dummy papers)
func SeedDevelopmentData(db *gorm.DB) error {
	// 1. Seed Roles (Only SUPER_ADMIN and STUDENT)
	roles := []models.Role{
		{Name: models.RoleSuperAdmin, Description: "Platform Super Administrator with complete system access"},
		{Name: models.RoleStudent, Description: "Student / Researcher submitting and tracking research papers"},
	}
	roleMap := make(map[string]string)
	for _, r := range roles {
		var existing models.Role
		if err := db.Where("name = ?", r.Name).First(&existing).Error; err != nil {
			_ = db.Create(&r).Error
			roleMap[r.Name] = r.ID
		} else {
			roleMap[r.Name] = existing.ID
		}
	}

	// 2. Seed Centralized Branding & System Settings
	defaultSettings := []models.SystemSetting{
		{SettingKey: "brand_name", SettingValue: "ResearchHub", SettingGroup: "BRANDING", Description: "Platform Brand Name", IsPublic: true},
		{SettingKey: "brand_tagline", SettingValue: "International Journal of Academic Research & Innovation", SettingGroup: "BRANDING", Description: "Platform Academic Tagline", IsPublic: true},
		{SettingKey: "primary_color", SettingValue: "#1e293b", SettingGroup: "BRANDING", Description: "Primary Navy Blue Color", IsPublic: true},
		{SettingKey: "accent_color", SettingValue: "#6d28d9", SettingGroup: "BRANDING", Description: "Accent Academic Purple Color", IsPublic: true},
		{SettingKey: "contact_email", SettingValue: "editorial@researchhub.org", SettingGroup: "BRANDING", Description: "Official Editorial Contact Email", IsPublic: true},
		{SettingKey: "contact_phone", SettingValue: "+1 (800) 555-0199", SettingGroup: "BRANDING", Description: "Editorial Office Contact Number", IsPublic: true},
		{SettingKey: "contact_address", SettingValue: "Academic Plaza, Suite 400, Cambridge Innovation District", SettingGroup: "BRANDING", Description: "Office Address", IsPublic: true},
		{SettingKey: "journal_name", SettingValue: "ResearchHub Transactions on Multidisciplinary Sciences", SettingGroup: "PUBLICATION", Description: "Default Journal Title", IsPublic: true},
		{SettingKey: "journal_issn", SettingValue: "2994-812X", SettingGroup: "PUBLICATION", Description: "Journal ISSN", IsPublic: true},
		{SettingKey: "current_volume", SettingValue: "12", SettingGroup: "PUBLICATION", Description: "Active Journal Volume", IsPublic: true},
		{SettingKey: "current_issue", SettingValue: "4", SettingGroup: "PUBLICATION", Description: "Active Journal Issue", IsPublic: true},
		{SettingKey: "signatory_name", SettingValue: "Prof. Dr. Eleanor Vance", SettingGroup: "PUBLICATION", Description: "Certificate Authorized Signatory", IsPublic: true},
		{SettingKey: "signatory_title", SettingValue: "Editor-in-Chief, ResearchHub Editorial Board", SettingGroup: "PUBLICATION", Description: "Signatory Designation", IsPublic: true},
		{SettingKey: "max_upload_size_mb", SettingValue: "25", SettingGroup: "SECURITY", Description: "Maximum PDF file size in MB", IsPublic: true},
	}
	for _, s := range defaultSettings {
		var existing models.SystemSetting
		if err := db.Where("setting_key = ?", s.SettingKey).First(&existing).Error; err != nil {
			_ = db.Create(&s).Error
		}
	}

	// 3. Seed Website Content Pages
	contents := []models.WebsiteContent{
		{
			PageKey:         "home",
			Title:           "Advancing Global Knowledge Through Rigorous Peer-Reviewed Research",
			Subtitle:        "ResearchHub is a premier academic publication platform empowering scholars, researchers, and institutions to publish high-impact peer-reviewed research.",
			ContentHTML:     "<p>Welcome to ResearchHub, a double-blind peer-reviewed academic publication management platform supporting transparent editorial workflows, verifiable digital certificates, and open-access research dissemination.</p>",
			MetaDescription: "ResearchHub - Peer-Reviewed Academic Research Paper Publication Management System",
		},
		{
			PageKey:         "about",
			Title:           "About ResearchHub Academic Press",
			Subtitle:        "Dedicated to excellence, integrity, and global accessibility in scholarly publishing.",
			ContentHTML:     "<p>ResearchHub provides an end-to-end digital scholarly publishing ecosystem connecting student researchers, faculty mentors, independent peer reviewers, and academic editors.</p><p>Every manuscript undergoes structured initial screening, multi-reviewer evaluation, iterative revision tracking, and formal editorial decision-making before publication and automated certificate issuance.</p>",
			MetaDescription: "Learn about ResearchHub's mission, editorial standards, and academic publishing impact.",
		},
		{
			PageKey:         "publication_guidelines",
			Title:           "Publication Guidelines & Editorial Policy",
			Subtitle:        "Standards and policies governing manuscript evaluation and journal publication.",
			ContentHTML:     "<h3>1. Peer Review Process</h3><p>All submitted manuscripts undergo initial editorial screening followed by evaluation from assigned domain reviewers based on originality, methodology, technical quality, and presentation.</p><h3>2. Revisions & Decisions</h3><p>Authors may be invited to submit revised manuscripts addressing reviewer feedback before the Editor makes a final decision.</p>",
			MetaDescription: "Publication guidelines and peer review policies for ResearchHub.",
		},
		{
			PageKey:         "submission_guidelines",
			Title:           "Research Paper Submission Guidelines",
			Subtitle:        "Formatting, structure, and originality requirements for prospective authors.",
			ContentHTML:     "<h3>Manuscript Formatting</h3><ul><li>Submit manuscripts in PDF format (up to 25 MB).</li><li>Include a concise title, structured abstract (150-300 words), and 4-6 indexing keywords.</li><li>List all co-authors with accurate institutional affiliations and designate a corresponding author.</li></ul>",
			MetaDescription: "Author instructions and manuscript preparation guidelines.",
		},
		{
			PageKey:         "ethics",
			Title:           "Publication Ethics & Malpractice Statement",
			Subtitle:        "Upholding academic honesty, originality, and conflict-of-interest transparency.",
			ContentHTML:     "<h3>Originality & Plagiarism</h3><p>Authors must confirm that submitted works are entirely original and not under consideration elsewhere.</p><h3>Fair Peer Review</h3><p>Reviewers evaluate manuscripts objectively and declare any conflict of interest. Confidential comments to the editor remain strictly protected.</p>",
			MetaDescription: "Publication ethics and academic integrity policies.",
		},
	}
	for _, c := range contents {
		var existing models.WebsiteContent
		if err := db.Where("page_key = ?", c.PageKey).First(&existing).Error; err != nil {
			_ = db.Create(&c).Error
		}
	}

	if config.Cfg != nil && !config.Cfg.SeedDevData {
		return nil
	}

	// 5. Seed Core Administrative & Editorial Staff Accounts (Super Admin, Editor, Reviewers)
	// Students register directly via the application and are never pre-populated with dummy accounts.
	hashPass := func(pwd string) string {
		h, _ := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
		return string(h)
	}

	type seedUser struct {
		Email       string
		Password    string
		FullName    string
		Role        string
		Phone       string
		Institution string
		Department  string
		Designation string
		Bio         string
	}

	staffUsers := []seedUser{
		{
			Email:       "admin@researchhub.org",
			Password:    "Admin@123",
			FullName:    "Dr. Marcus Sterling",
			Role:        models.RoleSuperAdmin,
			Phone:       "+1-617-555-0101",
			Institution: "ResearchHub Academic Press",
			Department:  "Executive Administration",
			Designation: "Chief Platform Administrator",
			Bio:         "Super Administrator overseeing academic publishing operations, compliance, and digital infrastructure.",
		},
		{
			Email:       "aaaaditya82@gmail.com",
			Password:    "Admin@123",
			FullName:    "Aditya Anerao",
			Role:        models.RoleSuperAdmin,
			Phone:       "+91-0000000000",
			Institution: "ResearchHub Academic Press",
			Department:  "Executive Administration",
			Designation: "Platform Administrator",
			Bio:         "Super Administrator for ResearchHub.",
		},
	}

	for _, u := range staffUsers {
		var existing models.User
		if err := db.Where("email = ?", u.Email).First(&existing).Error; err != nil {
			newUser := models.User{
				Email:        u.Email,
				PasswordHash: hashPass(u.Password),
				FullName:     u.FullName,
				RoleID:       roleMap[u.Role],
				RoleName:     u.Role,
				Phone:        u.Phone,
				Institution:  u.Institution,
				Department:   u.Department,
				Designation:  u.Designation,
				Bio:          u.Bio,
				IsActive:     true,
			}
			_ = db.Create(&newUser).Error
		}
	}

	log.Println("[INFO] Core system roles, settings, categories, and editorial staff verified.")
	return nil
}

// SeedEssentialCategories idempotently seeds default research categories in all environments (independent of SEED_DEV_DATA).
func SeedEssentialCategories(db *gorm.DB) error {
	categories := []models.ResearchCategory{
		{Name: "Artificial Intelligence & Machine Learning", Code: "AIML", Discipline: "Computer Science", Description: "Deep learning, NLP, computer vision, neural architectures, and autonomous systems.", IsActive: true},
		{Name: "Cybersecurity & Network Systems", Code: "CYBER", Discipline: "Information Security", Description: "Network defense, zero-trust architectures, post-quantum cryptography, and privacy.", IsActive: true},
		{Name: "Data Science & Cloud Computing", Code: "DSCC", Discipline: "Computer Engineering", Description: "Distributed systems, big data analytics, cloud-native orchestration, and edge computing.", IsActive: true},
		{Name: "Biomedical Engineering & Health Informatics", Code: "BMEH", Discipline: "Life Sciences & Healthcare", Description: "Medical imaging, clinical decision systems, bioinformatics, and wearable diagnostics.", IsActive: true},
		{Name: "Renewable Energy & Sustainable Systems", Code: "RESS", Discipline: "Environmental & Electrical Engineering", Description: "Smart grids, solar photovoltaics, energy storage optimization, and green technologies.", IsActive: true},
	}
	for _, cat := range categories {
		var existing models.ResearchCategory
		if err := db.Where("code = ?", cat.Code).First(&existing).Error; err != nil {
			if cErr := db.Create(&cat).Error; cErr != nil {
				return cErr
			}
		}
	}
	return nil
}
