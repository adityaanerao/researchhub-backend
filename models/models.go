package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Role constants
const (
	RoleStudent    = "STUDENT"
	RoleSuperAdmin = "SUPER_ADMIN"
)

// Paper Workflow Status constants
const (
	StatusDraft                      = "DRAFT"
	StatusSubmitted                  = "SUBMITTED"
	StatusInitialScreening           = "INITIAL_SCREENING"
	StatusUnderReview                = "UNDER_REVIEW"
	StatusRevisionRequired           = "REVISION_REQUIRED"
	StatusReviewRecommendedApproval  = "REVIEW_RECOMMENDED_APPROVAL"
	StatusReviewRecommendedRejection = "REVIEW_RECOMMENDED_REJECTION"
	StatusRevisionSubmitted          = "REVISION_SUBMITTED"
	StatusFinalEditorialDecision     = "FINAL_EDITORIAL_DECISION"
	StatusApproved                   = "APPROVED"
	StatusRejected                   = "REJECTED"
	StatusReadyForPublication        = "READY_FOR_PUBLICATION"
	StatusPublished                  = "PUBLISHED"
)

// BaseModel provides UUID primary key, timestamps, and soft deletion
type BaseModel struct {
	ID        string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (b *BaseModel) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	return nil
}

// 1. roles
type Role struct {
	BaseModel
	Name        string `gorm:"type:varchar(50);uniqueIndex;not null" json:"name"`
	Description string `gorm:"type:varchar(255)" json:"description"`
}

// 2. users
type User struct {
	BaseModel
	Email              string           `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	PasswordHash       string           `gorm:"type:varchar(255);not null" json:"-"`
	FullName           string           `gorm:"type:varchar(150);not null" json:"full_name"`
	RoleID             string           `gorm:"type:varchar(36);index" json:"role_id"`
	RoleName           string           `gorm:"type:varchar(50);index;not null" json:"role"`
	Phone              string           `gorm:"type:varchar(50)" json:"phone"`
	Institution        string           `gorm:"type:varchar(200);index" json:"institution"`
	Department         string           `gorm:"type:varchar(150)" json:"department"`
	Designation        string           `gorm:"type:varchar(150)" json:"designation"`
	Bio                string           `gorm:"type:text" json:"bio"`
	AvatarURL          string           `gorm:"type:varchar(255)" json:"avatar_url"`
	IsActive           bool             `gorm:"default:true;index" json:"is_active"`
	ResetToken         string           `gorm:"type:varchar(255);index" json:"-"`
	ResetTokenExpiry   *time.Time       `json:"-"`
	LastLoginAt        *time.Time       `json:"last_login_at"`
	StudentProfile     *StudentProfile  `gorm:"foreignKey:UserID" json:"student_profile,omitempty"`
	ReviewerProfile    *ReviewerProfile `gorm:"foreignKey:UserID" json:"reviewer_profile,omitempty"`
	EditorProfile      *EditorProfile   `gorm:"foreignKey:UserID" json:"editor_profile,omitempty"`
}

// 3. student_profiles
type StudentProfile struct {
	BaseModel
	UserID           string `gorm:"type:varchar(36);uniqueIndex;not null" json:"user_id"`
	EnrollmentNumber string `gorm:"type:varchar(100)" json:"enrollment_number"`
	DegreeProgram    string `gorm:"type:varchar(150)" json:"degree_program"`
	AcademicYear     string `gorm:"type:varchar(50)" json:"academic_year"`
	University       string `gorm:"type:varchar(200)" json:"university"`
	Institution      string `gorm:"type:varchar(200)" json:"institution"`
	Department       string `gorm:"type:varchar(150)" json:"department"`
	GuideName        string `gorm:"type:varchar(150)" json:"guide_name"`
	GuideEmail       string `gorm:"type:varchar(150)" json:"guide_email"`
	ResearchInterest string `gorm:"type:text" json:"research_interest"`
	ORCID            string `gorm:"type:varchar(100)" json:"orcid"`
}

// 4. reviewer_profiles
type ReviewerProfile struct {
	BaseModel
	UserID             string  `gorm:"type:varchar(36);uniqueIndex;not null" json:"user_id"`
	ExpertiseAreas     string  `gorm:"type:text" json:"expertise_areas"`
	Qualification      string  `gorm:"type:varchar(200)" json:"qualification"`
	ExperienceYears    int     `gorm:"default:0" json:"experience_years"`
	Institution        string  `gorm:"type:varchar(200)" json:"institution"`
	Department         string  `gorm:"type:varchar(150)" json:"department"`
	ORCID              string  `gorm:"type:varchar(100)" json:"orcid"`
	GoogleScholarURL   string  `gorm:"type:varchar(255)" json:"google_scholar_url"`
	MaxActiveReviews   int     `gorm:"default:5" json:"max_active_reviews"`
	TotalReviewsDone   int     `gorm:"default:0" json:"total_reviews_done"`
	AverageReviewScore float64 `gorm:"default:0" json:"average_review_score"`
}

// 5. editor_profiles
type EditorProfile struct {
	BaseModel
	UserID            string `gorm:"type:varchar(36);uniqueIndex;not null" json:"user_id"`
	EditorialRole     string `gorm:"type:varchar(150)" json:"editorial_role"` // e.g. Editor-in-Chief, Associate Editor
	Specialization    string `gorm:"type:text" json:"specialization"`
	Institution       string `gorm:"type:varchar(200)" json:"institution"`
	Department        string `gorm:"type:varchar(150)" json:"department"`
	Qualification     string `gorm:"type:varchar(200)" json:"qualification"`
	ORCID             string `gorm:"type:varchar(100)" json:"orcid"`
	PublicBoardMember bool   `gorm:"default:true" json:"public_board_member"`
	DisplayOrder      int    `gorm:"default:1" json:"display_order"`
}

// 6. research_categories
type ResearchCategory struct {
	BaseModel
	Name        string `gorm:"type:varchar(150);uniqueIndex;not null" json:"name"`
	Code        string `gorm:"type:varchar(50);uniqueIndex;not null" json:"code"`
	Description string `gorm:"type:text" json:"description"`
	Discipline  string `gorm:"type:varchar(150);index" json:"discipline"`
	IsActive    bool   `gorm:"default:true;index" json:"is_active"`
}

// 7. research_papers
type ResearchPaper struct {
	BaseModel
	SubmissionCode          string               `gorm:"type:varchar(50);uniqueIndex;not null" json:"submission_code"`
	Title                   string               `gorm:"type:varchar(350);not null;index" json:"title"`
	Abstract                string               `gorm:"type:text;not null" json:"abstract"`
	Keywords                string               `gorm:"type:varchar(500)" json:"keywords"`
	CategoryID              string               `gorm:"type:varchar(36);index;not null" json:"category_id"`
	Category                *ResearchCategory    `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	ResearchArea            string               `gorm:"type:varchar(200)" json:"research_area"`
	StudentID               string               `gorm:"type:varchar(36);index;not null" json:"student_id"`
	Student                 *User                `gorm:"foreignKey:StudentID" json:"student,omitempty"`
	CorrespondingAuthor     string               `gorm:"type:varchar(150);not null" json:"corresponding_author"`
	CorrespondingEmail      string               `gorm:"type:varchar(200);not null" json:"corresponding_email"`
	ContactNumber           string               `gorm:"type:varchar(50)" json:"contact_number"`
	Institution             string               `gorm:"type:varchar(200);index" json:"institution"`
	Department              string               `gorm:"type:varchar(150)" json:"department"`
	GuideName               string               `gorm:"type:varchar(150)" json:"guide_name"`
	DeclarationAccepted     bool                 `gorm:"default:true" json:"declaration_accepted"`
	OriginalityConfirmed    bool                 `gorm:"default:true" json:"originality_confirmed"`
	ProjectLogoURL          string               `gorm:"type:varchar(500)" json:"project_logo_url"`
	Status                  string               `gorm:"type:varchar(60);index;not null" json:"status"`
	CurrentRound            int                  `gorm:"default:1" json:"current_round"`
	AssignedEditorID        *string              `gorm:"type:varchar(36);index" json:"assigned_editor_id"`
	ScreeningNotes          string               `gorm:"type:text" json:"screening_notes"`
	EditorialNotes          string               `gorm:"type:text" json:"editorial_notes"`
	SubmittedAt             time.Time            `json:"submitted_at"`
	ApprovedAt              *time.Time           `json:"approved_at"`
	PublishedAt             *time.Time           `json:"published_at"`
	Authors                 []PaperAuthor        `gorm:"foreignKey:PaperID" json:"authors,omitempty"`
	Files                   []PaperFile          `gorm:"foreignKey:PaperID" json:"files,omitempty"`
	StatusHistory           []PaperStatusHistory `gorm:"foreignKey:PaperID" json:"status_history,omitempty"`
	Assignments             []PaperAssignment    `gorm:"foreignKey:PaperID" json:"assignments,omitempty"`
	Reviews                 []PaperReview        `gorm:"foreignKey:PaperID" json:"reviews,omitempty"`
	Revisions               []RevisionHistory    `gorm:"foreignKey:PaperID" json:"revisions,omitempty"`
	Decisions               []EditorialDecision  `gorm:"foreignKey:PaperID" json:"decisions,omitempty"`
	Publication             *Publication         `gorm:"foreignKey:PaperID" json:"publication,omitempty"`
}

// 8. paper_authors
type PaperAuthor struct {
	BaseModel
	PaperID         string `gorm:"type:varchar(36);index;not null" json:"paper_id"`
	FullName        string `gorm:"type:varchar(150);not null;index" json:"full_name"`
	Email           string `gorm:"type:varchar(200);not null" json:"email"`
	Phone           string `gorm:"type:varchar(50)" json:"phone"`
	Institution     string `gorm:"type:varchar(200)" json:"institution"`
	Department      string `gorm:"type:varchar(150)" json:"department"`
	Designation     string `gorm:"type:varchar(100)" json:"designation"`
	AuthorOrder     int    `gorm:"default:1" json:"author_order"`
	IsCorresponding bool   `gorm:"default:false" json:"is_corresponding"`
}

// 9. paper_files
type PaperFile struct {
	BaseModel
	PaperID        string `gorm:"type:varchar(36);index;not null" json:"paper_id"`
	FileType       string `gorm:"type:varchar(50);index;not null" json:"file_type"` // INITIAL_SUBMISSION, REVISION, PUBLISHED_PDF
	OriginalName   string `gorm:"type:varchar(255);not null" json:"original_name"`
	StoredFileName string `gorm:"type:varchar(255);not null" json:"stored_file_name"`
	StoragePath    string `gorm:"type:varchar(500);not null" json:"-"`
	MimeType       string `gorm:"type:varchar(100);not null" json:"mime_type"`
	FileSize       int64  `json:"file_size"`
	VersionNumber  int    `gorm:"default:1" json:"version_number"`
	UploadedByID   string `gorm:"type:varchar(36);index" json:"uploaded_by_id"`
}

// 10. paper_status_history
type PaperStatusHistory struct {
	BaseModel
	PaperID         string `gorm:"type:varchar(36);index;not null" json:"paper_id"`
	FromStatus      string `gorm:"type:varchar(60)" json:"from_status"`
	ToStatus        string `gorm:"type:varchar(60);not null" json:"to_status"`
	ChangedByUserID string `gorm:"type:varchar(36);index" json:"changed_by_user_id"`
	ChangedByName   string `gorm:"type:varchar(150)" json:"changed_by_name"`
	ChangedByRole   string `gorm:"type:varchar(50)" json:"changed_by_role"`
	Remarks         string `gorm:"type:text" json:"remarks"`
}

// 11. paper_assignments
type PaperAssignment struct {
	BaseModel
	PaperID      string         `gorm:"type:varchar(36);index;not null" json:"paper_id"`
	Paper        *ResearchPaper `gorm:"foreignKey:PaperID" json:"paper,omitempty"`
	ReviewerID   string         `gorm:"type:varchar(36);index;not null" json:"reviewer_id"`
	Reviewer     *User          `gorm:"foreignKey:ReviewerID" json:"reviewer,omitempty"`
	AssignedByID string         `gorm:"type:varchar(36);index" json:"assigned_by_id"`
	ReviewRound  int            `gorm:"default:1" json:"review_round"`
	Status       string         `gorm:"type:varchar(50);default:'ASSIGNED';index" json:"status"` // ASSIGNED, IN_PROGRESS, COMPLETED, DECLINED
	DueDate      time.Time      `json:"due_date"`
	CompletedAt  *time.Time     `json:"completed_at"`
	Notes        string         `gorm:"type:text" json:"notes"`
	Review       *PaperReview   `gorm:"foreignKey:AssignmentID" json:"review,omitempty"`
}

// 12. paper_reviews
type PaperReview struct {
	BaseModel
	AssignmentID        string          `gorm:"type:varchar(36);uniqueIndex;not null" json:"assignment_id"`
	PaperID             string          `gorm:"type:varchar(36);index;not null" json:"paper_id"`
	Paper               *ResearchPaper  `gorm:"foreignKey:PaperID" json:"paper,omitempty"`
	ReviewerID          string          `gorm:"type:varchar(36);index;not null" json:"reviewer_id"`
	Reviewer            *User           `gorm:"foreignKey:ReviewerID" json:"reviewer,omitempty"`
	ReviewRound         int             `gorm:"default:1" json:"review_round"`
	OverallScore        int             `gorm:"not null" json:"overall_score"`        // 1 to 10
	OriginalityScore    int             `gorm:"not null" json:"originality_score"`    // 1 to 10
	MethodologyScore    int             `gorm:"not null" json:"methodology_score"`    // 1 to 10
	TechnicalQuality    int             `gorm:"not null" json:"technical_quality"`    // 1 to 10
	PresentationScore   int             `gorm:"not null" json:"presentation_score"`   // 1 to 10
	CommentsToAuthor    string          `gorm:"type:text;not null" json:"comments_to_author"`
	ConfidentialToEditor string         `gorm:"type:text" json:"confidential_to_editor"`
	Recommendation      string          `gorm:"type:varchar(60);not null" json:"recommendation"` // ACCEPT, MINOR_REVISION, MAJOR_REVISION, REJECT
	SubmittedAt         time.Time       `json:"submitted_at"`
	DetailedComments    []ReviewComment `gorm:"foreignKey:ReviewID" json:"detailed_comments,omitempty"`
}

// 13. review_comments
type ReviewComment struct {
	BaseModel
	ReviewID        string `gorm:"type:varchar(36);index;not null" json:"review_id"`
	PaperID         string `gorm:"type:varchar(36);index;not null" json:"paper_id"`
	Section         string `gorm:"type:varchar(100)" json:"section"`
	CommentText     string `gorm:"type:text;not null" json:"comment_text"`
	IsConfidential  bool   `gorm:"default:false" json:"is_confidential"`
	CreatedByUserID string `gorm:"type:varchar(36)" json:"created_by_user_id"`
}

// 14. revision_history
type RevisionHistory struct {
	BaseModel
	PaperID          string     `gorm:"type:varchar(36);index;not null" json:"paper_id"`
	RevisionRound    int        `gorm:"default:1" json:"revision_round"`
	RequestedByID    string     `gorm:"type:varchar(36);index" json:"requested_by_id"`
	RequestComments  string     `gorm:"type:text" json:"request_comments"`
	AuthorResponse   string     `gorm:"type:text" json:"author_response"`
	ChangesSummary   string     `gorm:"type:text" json:"changes_summary"`
	RevisedFileID    string     `gorm:"type:varchar(36)" json:"revised_file_id"`
	RequestedAt      time.Time  `json:"requested_at"`
	SubmittedAt      *time.Time `json:"submitted_at"`
	Status           string     `gorm:"type:varchar(50);default:'PENDING'" json:"status"` // PENDING, SUBMITTED
}

// 15. editorial_decisions
type EditorialDecision struct {
	BaseModel
	PaperID           string    `gorm:"type:varchar(36);index;not null" json:"paper_id"`
	EditorID          string    `gorm:"type:varchar(36);index;not null" json:"editor_id"`
	EditorName        string    `gorm:"type:varchar(150)" json:"editor_name"`
	ReviewRound       int       `gorm:"default:1" json:"review_round"`
	Decision          string    `gorm:"type:varchar(60);not null" json:"decision"` // INITIAL_SCREENING_PASSED, REVISION_REQUIRED, APPROVED, REJECTED, READY_FOR_PUBLICATION
	EditorialComments string    `gorm:"type:text" json:"editorial_comments"`
	DecidedAt         time.Time `json:"decided_at"`
}

// 16. publication_issues
type PublicationIssue struct {
	BaseModel
	JournalName     string    `gorm:"type:varchar(200);not null" json:"journal_name"`
	Volume          string    `gorm:"type:varchar(50);not null" json:"volume"`
	Issue           string    `gorm:"type:varchar(50);not null" json:"issue"`
	PublicationYear int       `gorm:"index;not null" json:"publication_year"`
	ISSN            string    `gorm:"type:varchar(50)" json:"issn"`
	Title           string    `gorm:"type:varchar(200)" json:"title"`
	PublishedDate   time.Time `json:"published_date"`
	IsCurrent       bool      `gorm:"default:true" json:"is_current"`
}

// 17. publications
type Publication struct {
	BaseModel
	PublicationCode string            `gorm:"type:varchar(60);uniqueIndex;not null" json:"publication_code"`
	PaperID         string            `gorm:"type:varchar(36);uniqueIndex;not null" json:"paper_id"`
	Paper           *ResearchPaper    `gorm:"foreignKey:PaperID" json:"paper,omitempty"`
	IssueID         *string           `gorm:"type:varchar(36);index" json:"issue_id"`
	PublicationIssue *PublicationIssue `gorm:"foreignKey:IssueID" json:"publication_issue,omitempty"`
	PublicationTitle string           `gorm:"type:varchar(350);not null;index" json:"publication_title"`
	JournalName     string            `gorm:"type:varchar(200);not null" json:"journal_name"`
	Volume          string            `gorm:"type:varchar(50);not null" json:"volume"`
	Issue           string            `gorm:"type:varchar(50);not null" json:"issue"`
	PublicationYear int               `gorm:"index;not null" json:"publication_year"`
	PublicationDate time.Time         `gorm:"index" json:"publication_date"`
	DOI             string            `gorm:"type:varchar(150)" json:"doi"` // Optional, never faked
	ISSN            string            `gorm:"type:varchar(50)" json:"issn"` // Optional
	PageStart       int               `json:"page_start"`
	PageEnd         int               `json:"page_end"`
	PublishedPDFID  string            `gorm:"type:varchar(36)" json:"published_pdf_id"`
	PublicationURL  string            `gorm:"type:varchar(300)" json:"publication_url"`
	DownloadCount   int               `gorm:"default:0" json:"download_count"`
	ViewCount       int               `gorm:"default:0" json:"view_count"`
	IsPublished     bool              `gorm:"default:false;index" json:"is_published"`
	PublishedByID   string            `gorm:"type:varchar(36)" json:"published_by_id"`
}

// 18. certificates
type Certificate struct {
	BaseModel
	CertificateCode    string         `gorm:"type:varchar(80);uniqueIndex;not null" json:"certificate_code"`
	PublicationID      *string        `gorm:"type:varchar(36);index" json:"publication_id"`
	Publication        *Publication   `gorm:"foreignKey:PublicationID" json:"publication,omitempty"`
	PaperID            string         `gorm:"type:varchar(36);index;not null" json:"paper_id"`
	Paper              *ResearchPaper `gorm:"foreignKey:PaperID" json:"paper,omitempty"`
	StudentID          string         `gorm:"type:varchar(36);index" json:"student_id"`
	AuthorName         string         `gorm:"type:varchar(150);not null" json:"author_name"`
	AuthorEmail        string         `gorm:"type:varchar(200)" json:"author_email"`
	PaperTitle         string         `gorm:"type:varchar(350);not null" json:"paper_title"`
	PublicationName    string         `gorm:"type:varchar(200);not null" json:"publication_name"`
	PublicationCode    string         `gorm:"type:varchar(60);not null" json:"publication_code"`
	Volume             string         `gorm:"type:varchar(50)" json:"volume"`
	Issue              string         `gorm:"type:varchar(50)" json:"issue"`
	PublicationDate    time.Time      `json:"publication_date"`
	SignatoryName      string         `gorm:"type:varchar(150)" json:"signatory_name"`
	SignatoryTitle     string         `gorm:"type:varchar(150)" json:"signatory_title"`
	QRVerificationURL  string         `gorm:"type:varchar(350)" json:"qr_verification_url"`
	PDFStoragePath     string         `gorm:"type:varchar(500)" json:"-"`
	IsRevoked          bool           `gorm:"default:false;index" json:"is_revoked"`
	IssuedAt           time.Time      `json:"issued_at"`
}

// 19. certificate_verifications
type CertificateVerification struct {
	BaseModel
	CertificateID   string    `gorm:"type:varchar(36);index" json:"certificate_id"`
	CertificateCode string    `gorm:"type:varchar(80);index;not null" json:"certificate_code"`
	VerifiedAt      time.Time `json:"verified_at"`
	VerifierIP      string    `gorm:"type:varchar(60)" json:"verifier_ip"`
	UserAgent       string    `gorm:"type:varchar(255)" json:"user_agent"`
	IsValidResult   bool      `json:"is_valid_result"`
}

// 20. notifications
type Notification struct {
	BaseModel
	UserID      string     `gorm:"type:varchar(36);index;not null" json:"user_id"`
	Title       string     `gorm:"type:varchar(200);not null" json:"title"`
	Message     string     `gorm:"type:text;not null" json:"message"`
	Type        string     `gorm:"type:varchar(50);index" json:"type"` // SUBMISSION, ASSIGNMENT, REVIEW, REVISION, DECISION, PUBLICATION, CERTIFICATE
	ReferenceID string     `gorm:"type:varchar(36)" json:"reference_id"`
	IsRead      bool       `gorm:"default:false;index" json:"is_read"`
	ReadAt      *time.Time `json:"read_at"`
}

// 21. contact_messages
type ContactMessage struct {
	BaseModel
	FullName    string     `gorm:"type:varchar(150);not null" json:"full_name"`
	Email       string     `gorm:"type:varchar(200);not null" json:"email"`
	Phone       string     `gorm:"type:varchar(50)" json:"phone"`
	Subject     string     `gorm:"type:varchar(250);not null" json:"subject"`
	Message     string     `gorm:"type:text;not null" json:"message"`
	Status      string     `gorm:"type:varchar(50);default:'UNREAD';index" json:"status"` // UNREAD, READ, RESOLVED
	AdminReply  string     `gorm:"type:text" json:"admin_reply"`
	RepliedAt   *time.Time `json:"replied_at"`
}

// 22. website_contents
type WebsiteContent struct {
	BaseModel
	PageKey         string `gorm:"type:varchar(100);uniqueIndex;not null" json:"page_key"` // home, about, publication_guidelines, submission_guidelines, ethics, contact
	Title           string `gorm:"type:varchar(250);not null" json:"title"`
	Subtitle        string `gorm:"type:varchar(350)" json:"subtitle"`
	ContentHTML     string `gorm:"type:text;not null" json:"content_html"`
	MetaDescription string `gorm:"type:varchar(350)" json:"meta_description"`
	UpdatedByUserID string `gorm:"type:varchar(36)" json:"updated_by_user_id"`
}

// 23. system_settings
type SystemSetting struct {
	BaseModel
	SettingKey   string `gorm:"type:varchar(100);uniqueIndex;not null" json:"setting_key"`
	SettingValue string `gorm:"type:text;not null" json:"setting_value"`
	SettingGroup string `gorm:"type:varchar(80);index" json:"setting_group"` // BRANDING, PUBLICATION, EMAIL, SECURITY
	Description  string `gorm:"type:varchar(255)" json:"description"`
	IsPublic     bool   `gorm:"default:true" json:"is_public"`
}

// 25. email_otps
type EmailOTP struct {
	BaseModel
	Email     string    `gorm:"type:varchar(255);index;not null" json:"email"`
	OTP       string    `gorm:"type:varchar(10);not null" json:"otp"`
	Purpose   string    `gorm:"type:varchar(50);not null" json:"purpose"` // REGISTER, FORGOT_PASSWORD
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `gorm:"default:false" json:"used"`
}

// 26. payments
type Payment struct {
	BaseModel
	ReceiptNo         string     `gorm:"type:varchar(60);uniqueIndex;not null" json:"receipt_no"`
	StudentID         string     `gorm:"type:varchar(36);index;not null" json:"student_id"`
	StudentName       string     `gorm:"type:varchar(150)" json:"student_name"`
	StudentEmail      string     `gorm:"type:varchar(200)" json:"student_email"`
	PaperTitle        string     `gorm:"type:varchar(350)" json:"paper_title"`
	AmountINR         int        `gorm:"default:1000" json:"amount_inr"`
	Currency          string     `gorm:"type:varchar(10);default:'INR'" json:"currency"`
	RazorpayOrderID   string     `gorm:"type:varchar(100);index" json:"razorpay_order_id"`
	RazorpayPaymentID string     `gorm:"type:varchar(100);index" json:"razorpay_payment_id"`
	RazorpaySignature string     `gorm:"type:varchar(255)" json:"-"`
	Status            string     `gorm:"type:varchar(30);default:'CREATED';index" json:"status"` // CREATED, PAID
	BillPDFPath       string     `gorm:"type:varchar(500)" json:"-"`
	PaidAt            *time.Time `json:"paid_at"`
}
// 24. audit_logs
type AuditLog struct {
	BaseModel
	UserID     string `gorm:"type:varchar(36);index" json:"user_id"`
	UserEmail  string `gorm:"type:varchar(200)" json:"user_email"`
	UserRole   string `gorm:"type:varchar(50)" json:"user_role"`
	Action     string `gorm:"type:varchar(150);index;not null" json:"action"`
	EntityType string `gorm:"type:varchar(100);index" json:"entity_type"`
	EntityID   string `gorm:"type:varchar(36);index" json:"entity_id"`
	Details    string `gorm:"type:text" json:"details"`
	IPAddress  string `gorm:"type:varchar(60)" json:"ip_address"`
}
