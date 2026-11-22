-- ResearchHub PostgreSQL Initial Schema Migration (24 Normalized Tables)
-- Uses UUID primary keys, foreign key relationships, timestamps, indexes, and soft deletion.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 1. roles
CREATE TABLE IF NOT EXISTS roles (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL,
    description VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 2. users
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    full_name VARCHAR(150) NOT NULL,
    role_id VARCHAR(36) REFERENCES roles(id),
    role_name VARCHAR(50) NOT NULL,
    phone VARCHAR(50),
    institution VARCHAR(200),
    department VARCHAR(150),
    designation VARCHAR(150),
    bio TEXT,
    avatar_url VARCHAR(255),
    is_active BOOLEAN DEFAULT TRUE,
    reset_token VARCHAR(255),
    reset_token_expiry TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 3. student_profiles
CREATE TABLE IF NOT EXISTS student_profiles (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    enrollment_number VARCHAR(100),
    degree_program VARCHAR(150),
    academic_year VARCHAR(50),
    institution VARCHAR(200),
    department VARCHAR(150),
    guide_name VARCHAR(150),
    guide_email VARCHAR(150),
    research_interest TEXT,
    orcid VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 4. reviewer_profiles
CREATE TABLE IF NOT EXISTS reviewer_profiles (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expertise_areas TEXT,
    qualification VARCHAR(200),
    experience_years INT DEFAULT 0,
    institution VARCHAR(200),
    department VARCHAR(150),
    orcid VARCHAR(100),
    google_scholar_url VARCHAR(255),
    max_active_reviews INT DEFAULT 5,
    total_reviews_done INT DEFAULT 0,
    average_review_score NUMERIC(4,2) DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 5. editor_profiles
CREATE TABLE IF NOT EXISTS editor_profiles (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    editorial_role VARCHAR(150),
    specialization TEXT,
    institution VARCHAR(200),
    department VARCHAR(150),
    qualification VARCHAR(200),
    orcid VARCHAR(100),
    public_board_member BOOLEAN DEFAULT TRUE,
    display_order INT DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 6. research_categories
CREATE TABLE IF NOT EXISTS research_categories (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(150) UNIQUE NOT NULL,
    code VARCHAR(50) UNIQUE NOT NULL,
    description TEXT,
    discipline VARCHAR(150),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 7. research_papers
CREATE TABLE IF NOT EXISTS research_papers (
    id VARCHAR(36) PRIMARY KEY,
    submission_code VARCHAR(50) UNIQUE NOT NULL,
    title VARCHAR(350) NOT NULL,
    abstract TEXT NOT NULL,
    keywords VARCHAR(500),
    category_id VARCHAR(36) NOT NULL REFERENCES research_categories(id),
    research_area VARCHAR(200),
    student_id VARCHAR(36) NOT NULL REFERENCES users(id),
    corresponding_author VARCHAR(150) NOT NULL,
    corresponding_email VARCHAR(200) NOT NULL,
    contact_number VARCHAR(50),
    institution VARCHAR(200),
    department VARCHAR(150),
    guide_name VARCHAR(150),
    declaration_accepted BOOLEAN DEFAULT TRUE,
    originality_confirmed BOOLEAN DEFAULT TRUE,
    status VARCHAR(60) NOT NULL,
    current_round INT DEFAULT 1,
    assigned_editor_id VARCHAR(36),
    screening_notes TEXT,
    editorial_notes TEXT,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    approved_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 8. paper_authors
CREATE TABLE IF NOT EXISTS paper_authors (
    id VARCHAR(36) PRIMARY KEY,
    paper_id VARCHAR(36) NOT NULL REFERENCES research_papers(id) ON DELETE CASCADE,
    full_name VARCHAR(150) NOT NULL,
    email VARCHAR(200) NOT NULL,
    phone VARCHAR(50),
    institution VARCHAR(200),
    department VARCHAR(150),
    designation VARCHAR(100),
    author_order INT DEFAULT 1,
    is_corresponding BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 9. paper_files
CREATE TABLE IF NOT EXISTS paper_files (
    id VARCHAR(36) PRIMARY KEY,
    paper_id VARCHAR(36) NOT NULL REFERENCES research_papers(id) ON DELETE CASCADE,
    file_type VARCHAR(50) NOT NULL,
    original_name VARCHAR(255) NOT NULL,
    stored_file_name VARCHAR(255) NOT NULL,
    storage_path VARCHAR(500) NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    file_size BIGINT NOT NULL,
    version_number INT DEFAULT 1,
    uploaded_by_id VARCHAR(36),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 10. paper_status_history
CREATE TABLE IF NOT EXISTS paper_status_history (
    id VARCHAR(36) PRIMARY KEY,
    paper_id VARCHAR(36) NOT NULL REFERENCES research_papers(id) ON DELETE CASCADE,
    from_status VARCHAR(60),
    to_status VARCHAR(60) NOT NULL,
    changed_by_user_id VARCHAR(36),
    changed_by_name VARCHAR(150),
    changed_by_role VARCHAR(50),
    remarks TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 11. paper_assignments
CREATE TABLE IF NOT EXISTS paper_assignments (
    id VARCHAR(36) PRIMARY KEY,
    paper_id VARCHAR(36) NOT NULL REFERENCES research_papers(id) ON DELETE CASCADE,
    reviewer_id VARCHAR(36) NOT NULL REFERENCES users(id),
    assigned_by_id VARCHAR(36),
    review_round INT DEFAULT 1,
    status VARCHAR(50) DEFAULT 'ASSIGNED',
    due_date TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 12. paper_reviews
CREATE TABLE IF NOT EXISTS paper_reviews (
    id VARCHAR(36) PRIMARY KEY,
    assignment_id VARCHAR(36) UNIQUE NOT NULL REFERENCES paper_assignments(id) ON DELETE CASCADE,
    paper_id VARCHAR(36) NOT NULL REFERENCES research_papers(id) ON DELETE CASCADE,
    reviewer_id VARCHAR(36) NOT NULL REFERENCES users(id),
    review_round INT DEFAULT 1,
    overall_score INT NOT NULL,
    originality_score INT NOT NULL,
    methodology_score INT NOT NULL,
    technical_quality INT NOT NULL,
    presentation_score INT NOT NULL,
    comments_to_author TEXT NOT NULL,
    confidential_to_editor TEXT,
    recommendation VARCHAR(60) NOT NULL,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 13. review_comments
CREATE TABLE IF NOT EXISTS review_comments (
    id VARCHAR(36) PRIMARY KEY,
    review_id VARCHAR(36) NOT NULL REFERENCES paper_reviews(id) ON DELETE CASCADE,
    paper_id VARCHAR(36) NOT NULL REFERENCES research_papers(id) ON DELETE CASCADE,
    section VARCHAR(100),
    comment_text TEXT NOT NULL,
    is_confidential BOOLEAN DEFAULT FALSE,
    created_by_user_id VARCHAR(36),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 14. revision_history
CREATE TABLE IF NOT EXISTS revision_history (
    id VARCHAR(36) PRIMARY KEY,
    paper_id VARCHAR(36) NOT NULL REFERENCES research_papers(id) ON DELETE CASCADE,
    revision_round INT DEFAULT 1,
    requested_by_id VARCHAR(36),
    request_comments TEXT,
    author_response TEXT,
    changes_summary TEXT,
    revised_file_id VARCHAR(36),
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    submitted_at TIMESTAMPTZ,
    status VARCHAR(50) DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 15. editorial_decisions
CREATE TABLE IF NOT EXISTS editorial_decisions (
    id VARCHAR(36) PRIMARY KEY,
    paper_id VARCHAR(36) NOT NULL REFERENCES research_papers(id) ON DELETE CASCADE,
    editor_id VARCHAR(36) NOT NULL REFERENCES users(id),
    editor_name VARCHAR(150),
    review_round INT DEFAULT 1,
    decision VARCHAR(60) NOT NULL,
    editorial_comments TEXT,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 16. publication_issues
CREATE TABLE IF NOT EXISTS publication_issues (
    id VARCHAR(36) PRIMARY KEY,
    journal_name VARCHAR(200) NOT NULL,
    volume VARCHAR(50) NOT NULL,
    issue VARCHAR(50) NOT NULL,
    publication_year INT NOT NULL,
    issn VARCHAR(50),
    title VARCHAR(200),
    published_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_current BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 17. publications
CREATE TABLE IF NOT EXISTS publications (
    id VARCHAR(36) PRIMARY KEY,
    publication_code VARCHAR(60) UNIQUE NOT NULL,
    paper_id VARCHAR(36) UNIQUE NOT NULL REFERENCES research_papers(id),
    issue_id VARCHAR(36) REFERENCES publication_issues(id),
    publication_title VARCHAR(350) NOT NULL,
    journal_name VARCHAR(200) NOT NULL,
    volume VARCHAR(50) NOT NULL,
    issue VARCHAR(50) NOT NULL,
    publication_year INT NOT NULL,
    publication_date TIMESTAMPTZ NOT NULL,
    doi VARCHAR(150),
    issn VARCHAR(50),
    page_start INT DEFAULT 1,
    page_end INT DEFAULT 10,
    published_pdf_id VARCHAR(36),
    publication_url VARCHAR(300),
    download_count INT DEFAULT 0,
    view_count INT DEFAULT 0,
    is_published BOOLEAN DEFAULT FALSE,
    published_by_id VARCHAR(36),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 18. certificates
CREATE TABLE IF NOT EXISTS certificates (
    id VARCHAR(36) PRIMARY KEY,
    certificate_code VARCHAR(80) UNIQUE NOT NULL,
    publication_id VARCHAR(36) NOT NULL REFERENCES publications(id),
    paper_id VARCHAR(36) NOT NULL REFERENCES research_papers(id),
    student_id VARCHAR(36),
    author_name VARCHAR(150) NOT NULL,
    author_email VARCHAR(200),
    paper_title VARCHAR(350) NOT NULL,
    publication_name VARCHAR(200) NOT NULL,
    publication_code VARCHAR(60) NOT NULL,
    volume VARCHAR(50),
    issue VARCHAR(50),
    publication_date TIMESTAMPTZ NOT NULL,
    signatory_name VARCHAR(150),
    signatory_title VARCHAR(150),
    qr_verification_url VARCHAR(350),
    pdf_storage_path VARCHAR(500),
    is_revoked BOOLEAN DEFAULT FALSE,
    issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 19. certificate_verifications
CREATE TABLE IF NOT EXISTS certificate_verifications (
    id VARCHAR(36) PRIMARY KEY,
    certificate_id VARCHAR(36),
    certificate_code VARCHAR(80) NOT NULL,
    verified_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    verifier_ip VARCHAR(60),
    user_agent VARCHAR(255),
    is_valid_result BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 20. notifications
CREATE TABLE IF NOT EXISTS notifications (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    message TEXT NOT NULL,
    type VARCHAR(50),
    reference_id VARCHAR(36),
    is_read BOOLEAN DEFAULT FALSE,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 21. contact_messages
CREATE TABLE IF NOT EXISTS contact_messages (
    id VARCHAR(36) PRIMARY KEY,
    full_name VARCHAR(150) NOT NULL,
    email VARCHAR(200) NOT NULL,
    phone VARCHAR(50),
    subject VARCHAR(250) NOT NULL,
    message TEXT NOT NULL,
    status VARCHAR(50) DEFAULT 'UNREAD',
    admin_reply TEXT,
    replied_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 22. website_contents
CREATE TABLE IF NOT EXISTS website_contents (
    id VARCHAR(36) PRIMARY KEY,
    page_key VARCHAR(100) UNIQUE NOT NULL,
    title VARCHAR(250) NOT NULL,
    subtitle VARCHAR(350),
    content_html TEXT NOT NULL,
    meta_description VARCHAR(350),
    updated_by_user_id VARCHAR(36),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 23. system_settings
CREATE TABLE IF NOT EXISTS system_settings (
    id VARCHAR(36) PRIMARY KEY,
    setting_key VARCHAR(100) UNIQUE NOT NULL,
    setting_value TEXT NOT NULL,
    setting_group VARCHAR(80),
    description VARCHAR(255),
    is_public BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 24. audit_logs
CREATE TABLE IF NOT EXISTS audit_logs (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36),
    user_email VARCHAR(200),
    user_role VARCHAR(50),
    action VARCHAR(150) NOT NULL,
    entity_type VARCHAR(100),
    entity_id VARCHAR(36),
    details TEXT,
    ip_address VARCHAR(60),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
