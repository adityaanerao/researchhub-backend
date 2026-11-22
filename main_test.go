package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"researchhub-backend/config"
	"researchhub-backend/migrations"
	"researchhub-backend/models"
	"researchhub-backend/routes"
	"researchhub-backend/services"

	"github.com/gin-gonic/gin"
)

func setupTestApp(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	cfg := &config.AppConfig{
		Port:                    "8080",
		AppEnv:                  "test",
		AppName:                 "ResearchHub",
		AppURL:                  "http://localhost:4200",
		APIURL:                  "http://localhost:8080/api/v1",
		DBDriver:                "sqlite",
		SQLitePath:              filepath.Join(tmpDir, "test_researchhub.db"),
		JWTSecret:               "test_secret_key_for_unit_and_integration_tests",
		JWTExpiryHours:          24,
		ResetTokenExpiryMinutes: 60,
		StorageDriver:           "local",
		StorageLocalPath:        filepath.Join(tmpDir, "storage"),
		MaxUploadSizeMB:         25,
		SeedDevData:             true,
	}
	config.Cfg = cfg
	services.InitStorageService(cfg)

	db, err := config.ConnectDatabase(cfg)
	if err != nil {
		t.Fatalf("Failed to connect test database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := migrations.RunMigrations(db); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}
	if err := migrations.SeedDevelopmentData(db); err != nil {
		t.Fatalf("Failed to seed development data: %v", err)
	}

	return routes.SetupRouter()
}

func loginTestUser(t *testing.T, r *gin.Engine, email, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Login failed for %s: status=%d body=%s", email, w.Code, w.Body.String())
	}

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Success || resp.Data.Token == "" {
		t.Fatalf("Expected valid JWT token for %s", email)
	}
	return resp.Data.Token
}

func TestEndToEndResearchPublicationLifecycle(t *testing.T) {
	r := setupTestApp(t)

	// 1. Verify Non-Student Public Registration is Blocked
	badRegBody, _ := json.Marshal(map[string]string{
		"full_name":   "Unauthorized Reviewer",
		"email":       "hacker@example.com",
		"password":    "Secret@123",
		"institution": "Test Uni",
		"role":        "SUPER_ADMIN",
	})
	reqBadReg, _ := http.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(badRegBody))
	reqBadReg.Header.Set("Content-Type", "application/json")
	wBadReg := httptest.NewRecorder()
	r.ServeHTTP(wBadReg, reqBadReg)
	if wBadReg.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden for public SUPER_ADMIN registration, got %d", wBadReg.Code)
	}

	// 2. Verify Valid Student Registration
	regBody, _ := json.Marshal(map[string]string{
		"full_name":      "Priya Nair",
		"email":          "priya.nair@researchhub.org",
		"password":       "Student@123",
		"institution":    "Indian Institute of Science",
		"department":     "Computer Science",
		"degree_program": "Ph.D. Scholar",
	})
	reqReg, _ := http.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(regBody))
	reqReg.Header.Set("Content-Type", "application/json")
	wReg := httptest.NewRecorder()
	r.ServeHTTP(wReg, reqReg)
	if wReg.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created for student registration, got %d: %s", wReg.Code, wReg.Body.String())
	}

	// 3. Login as Student, Reviewer, Editor, and Super Admin
	studentToken := loginTestUser(t, r, "priya.nair@researchhub.org", "Student@123")
	reviewerToken := loginTestUser(t, r, "reviewer1@researchhub.org", "Reviewer@123")
	editorToken := loginTestUser(t, r, "editor@researchhub.org", "Editor@123")
	adminToken := loginTestUser(t, r, "admin@researchhub.org", "Admin@123")

	// 4. Verify Role-Based Access Control (Student cannot access Admin endpoint)
	reqRBAC, _ := http.NewRequest("GET", "/api/v1/admin/dashboard", nil)
	reqRBAC.Header.Set("Authorization", "Bearer "+studentToken)
	wRBAC := httptest.NewRecorder()
	r.ServeHTTP(wRBAC, reqRBAC)
	if wRBAC.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when student accesses admin dashboard, got %d", wRBAC.Code)
	}

	// 5. Submit Research Paper with Multiple Authors & Real PDF Upload
	var cat models.ResearchCategory
	config.DB.First(&cat)

	var bodyBuf bytes.Buffer
	mw := multipart.NewWriter(&bodyBuf)
	_ = mw.WriteField("title", "Quantum-Resistant Neural Cryptography for Distributed Autonomous Grids")
	_ = mw.WriteField("abstract", "This paper introduces a novel lattice-backed neural key exchange mechanism resilient against quantum adversaries.")
	_ = mw.WriteField("keywords", "Quantum Cryptography, Neural Key Exchange, Smart Grids")
	_ = mw.WriteField("category_id", cat.ID)
	_ = mw.WriteField("research_area", "Quantum Information Security")
	_ = mw.WriteField("corresponding_author", "Priya Nair")
	_ = mw.WriteField("corresponding_email", "priya.nair@researchhub.org")
	_ = mw.WriteField("contact_number", "+91-9876543210")
	_ = mw.WriteField("institution", "Indian Institute of Science")
	_ = mw.WriteField("department", "Computer Science")
	_ = mw.WriteField("guide_name", "Prof. K. Subramanian")
	_ = mw.WriteField("declaration_accepted", "true")
	_ = mw.WriteField("originality_confirmed", "true")
	_ = mw.WriteField("authors", `[{"full_name":"Priya Nair","email":"priya.nair@researchhub.org","institution":"Indian Institute of Science","author_order":1,"is_corresponding":true},{"full_name":"Prof. K. Subramanian","email":"subramanian@iisc.ac.in","institution":"Indian Institute of Science","author_order":2,"is_corresponding":false}]`)

	fw, _ := mw.CreateFormFile("pdf_file", "manuscript.pdf")
	_, _ = fw.Write([]byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n"))
	_ = mw.Close()

	reqSub, _ := http.NewRequest("POST", "/api/v1/student/papers", &bodyBuf)
	reqSub.Header.Set("Authorization", "Bearer "+studentToken)
	reqSub.Header.Set("Content-Type", mw.FormDataContentType())
	wSub := httptest.NewRecorder()
	r.ServeHTTP(wSub, reqSub)
	if wSub.Code != http.StatusCreated {
		t.Fatalf("Paper submission failed: status=%d body=%s", wSub.Code, wSub.Body.String())
	}

	var subResp struct {
		Data models.ResearchPaper `json:"data"`
	}
	_ = json.Unmarshal(wSub.Body.Bytes(), &subResp)
	paperID := subResp.Data.ID
	if paperID == "" || subResp.Data.SubmissionCode == "" {
		t.Fatalf("Expected generated paper ID and submission code")
	}

	// 6. Editor Performs Initial Screening -> Assigns Reviewer
	screenBody, _ := json.Marshal(map[string]string{
		"action":          "PASS_SCREENING",
		"screening_notes": "Formatting and originality verified.",
	})
	reqScreen, _ := http.NewRequest("POST", "/api/v1/editor/papers/"+paperID+"/screening", bytes.NewReader(screenBody))
	reqScreen.Header.Set("Authorization", "Bearer "+editorToken)
	reqScreen.Header.Set("Content-Type", "application/json")
	wScreen := httptest.NewRecorder()
	r.ServeHTTP(wScreen, reqScreen)
	if wScreen.Code != http.StatusOK {
		t.Fatalf("Initial screening failed: %d %s", wScreen.Code, wScreen.Body.String())
	}

	var rev1 models.User
	config.DB.Where("email = ?", "reviewer1@researchhub.org").First(&rev1)

	assignBody, _ := json.Marshal(map[string]interface{}{
		"reviewer_ids": []string{rev1.ID},
		"due_date":     "2026-12-15",
		"notes":        "Please evaluate mathematical security proof.",
	})
	reqAssign, _ := http.NewRequest("POST", "/api/v1/editor/papers/"+paperID+"/assign-reviewers", bytes.NewReader(assignBody))
	reqAssign.Header.Set("Authorization", "Bearer "+editorToken)
	reqAssign.Header.Set("Content-Type", "application/json")
	wAssign := httptest.NewRecorder()
	r.ServeHTTP(wAssign, reqAssign)
	if wAssign.Code != http.StatusOK {
		t.Fatalf("Reviewer assignment failed: %d %s", wAssign.Code, wAssign.Body.String())
	}

	// 7. Reviewer Submits Peer Review
	var assignment models.PaperAssignment
	config.DB.Where("paper_id = ? AND reviewer_id = ?", paperID, rev1.ID).First(&assignment)

	revSubmitBody, _ := json.Marshal(map[string]interface{}{
		"overall_score":          8,
		"originality_score":      9,
		"methodology_score":      8,
		"technical_quality":      8,
		"presentation_score":     8,
		"comments_to_author":     "Well-structured manuscript. Please clarify theorem 2 proof bounds.",
		"confidential_to_editor": "Recommend minor revision followed by acceptance.",
		"recommendation":         "MINOR_REVISION",
	})
	reqRev, _ := http.NewRequest("POST", "/api/v1/reviewer/assignments/"+assignment.ID+"/review", bytes.NewReader(revSubmitBody))
	reqRev.Header.Set("Authorization", "Bearer "+reviewerToken)
	reqRev.Header.Set("Content-Type", "application/json")
	wRev := httptest.NewRecorder()
	r.ServeHTTP(wRev, reqRev)
	if wRev.Code != http.StatusOK {
		t.Fatalf("Review submission failed: %d %s", wRev.Code, wRev.Body.String())
	}

	// 8. Editor Requests Revision -> Student Uploads Revised Paper
	reqReviseBody, _ := json.Marshal(map[string]string{
		"comments": "Please address Reviewer 1's comment on Theorem 2 proof bounds.",
	})
	reqEdRev, _ := http.NewRequest("POST", "/api/v1/editor/papers/"+paperID+"/request-revision", bytes.NewReader(reqReviseBody))
	reqEdRev.Header.Set("Authorization", "Bearer "+editorToken)
	reqEdRev.Header.Set("Content-Type", "application/json")
	wEdRev := httptest.NewRecorder()
	r.ServeHTTP(wEdRev, reqEdRev)
	if wEdRev.Code != http.StatusOK {
		t.Fatalf("Request revision failed: %d %s", wEdRev.Code, wEdRev.Body.String())
	}

	var revBuf bytes.Buffer
	rmw := multipart.NewWriter(&revBuf)
	_ = rmw.WriteField("author_response", "We have expanded Appendix A with the full proof for Theorem 2.")
	_ = rmw.WriteField("changes_summary", "Added Theorem 2 lemma proof and updated latency comparison table.")
	rfw, _ := rmw.CreateFormFile("pdf_file", "manuscript_revised_v2.pdf")
	_, _ = rfw.Write([]byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n"))
	_ = rmw.Close()

	reqStuRev, _ := http.NewRequest("POST", "/api/v1/student/papers/"+paperID+"/revisions", &revBuf)
	reqStuRev.Header.Set("Authorization", "Bearer "+studentToken)
	reqStuRev.Header.Set("Content-Type", rmw.FormDataContentType())
	wStuRev := httptest.NewRecorder()
	r.ServeHTTP(wStuRev, reqStuRev)
	if wStuRev.Code != http.StatusOK {
		t.Fatalf("Student revision upload failed: %d %s", wStuRev.Code, wStuRev.Body.String())
	}

	// 9. Editor Makes Final Decision: APPROVED
	decBody, _ := json.Marshal(map[string]string{
		"decision": "APPROVED",
		"comments": "All reviewer comments addressed satisfactorily. Approved for publication.",
	})
	reqDec, _ := http.NewRequest("POST", "/api/v1/editor/papers/"+paperID+"/decision", bytes.NewReader(decBody))
	reqDec.Header.Set("Authorization", "Bearer "+editorToken)
	reqDec.Header.Set("Content-Type", "application/json")
	wDec := httptest.NewRecorder()
	r.ServeHTTP(wDec, reqDec)
	if wDec.Code != http.StatusOK {
		t.Fatalf("Editorial approval failed: %d %s", wDec.Code, wDec.Body.String())
	}

	// 10. Publish Approved Paper & Verify Automatic Multi-Author Certificate Generation
	pubBody, _ := json.Marshal(map[string]interface{}{
		"paper_id":         paperID,
		"journal_name":     "ResearchHub Transactions on Multidisciplinary Sciences",
		"volume":           "12",
		"issue":            "4",
		"publication_year": 2026,
		"page_start":       15,
		"page_end":         28,
		"publish_now":      true,
	})
	reqPub, _ := http.NewRequest("POST", "/api/v1/admin/publications", bytes.NewReader(pubBody))
	reqPub.Header.Set("Authorization", "Bearer "+editorToken)
	reqPub.Header.Set("Content-Type", "application/json")
	wPub := httptest.NewRecorder()
	r.ServeHTTP(wPub, reqPub)
	if wPub.Code != http.StatusCreated {
		t.Fatalf("Publication failed: %d %s", wPub.Code, wPub.Body.String())
	}

	var pubResp struct {
		Data struct {
			Publication  models.Publication   `json:"publication"`
			Certificates []models.Certificate `json:"certificates"`
		} `json:"data"`
	}
	_ = json.Unmarshal(wPub.Body.Bytes(), &pubResp)
	if len(pubResp.Data.Certificates) != 2 {
		t.Fatalf("Expected 2 individual certificates for the 2 co-authors, got %d", len(pubResp.Data.Certificates))
	}

	// Verify DOI is not faked when omitted
	if pubResp.Data.Publication.DOI != "" {
		t.Fatalf("Expected empty DOI when not provided, got %s", pubResp.Data.Publication.DOI)
	}

	// Verify PDF certificate exists on disk and can be downloaded via API
	certCode := pubResp.Data.Certificates[0].CertificateCode
	var dbCert models.Certificate
	config.DB.Where("certificate_code = ?", certCode).First(&dbCert)
	if _, err := os.Stat(dbCert.PDFStoragePath); err != nil {
		t.Fatalf("Expected generated certificate PDF file on disk: %v", err)
	}
	reqCertDL, _ := http.NewRequest("GET", "/api/v1/certificates/"+certCode+"/download", nil)
	wCertDL := httptest.NewRecorder()
	r.ServeHTTP(wCertDL, reqCertDL)
	if wCertDL.Code != http.StatusOK || !bytes.HasPrefix(wCertDL.Body.Bytes(), []byte("%PDF-1.4")) {
		t.Fatalf("Expected valid PDF download for certificate %s", certCode)
	}

	// 11. Verify Public Certificate Verification Route (/api/v1/public/certificates/verify/:certificateId)
	reqVerify, _ := http.NewRequest("GET", "/api/v1/public/certificates/verify/"+certCode, nil)
	wVerify := httptest.NewRecorder()
	r.ServeHTTP(wVerify, reqVerify)
	if wVerify.Code != http.StatusOK {
		t.Fatalf("Public certificate verification failed: %d", wVerify.Code)
	}
	var verifyResp struct {
		Data map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(wVerify.Body.Bytes(), &verifyResp)
	if verifyResp.Data["is_valid"] != true || verifyResp.Data["status"] != "VALID" {
		t.Fatalf("Expected certificate to verify as VALID, got %v", verifyResp.Data)
	}

	// 12. Verify Super Admin Reports (All 13 reports + PDF & Excel export)
	reportTypes := []string{
		"student_registration", "paper_submission", "pending_review", "reviewer_performance",
		"revision", "approved_paper", "rejected_paper", "published_paper",
		"certificate_generation", "monthly_publication", "yearly_publication",
		"category_wise", "institution_wise",
	}
	for _, rt := range reportTypes {
		reqRep, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/admin/reports?report_type=%s", rt), nil)
		reqRep.Header.Set("Authorization", "Bearer "+adminToken)
		wRep := httptest.NewRecorder()
		r.ServeHTTP(wRep, reqRep)
		if wRep.Code != http.StatusOK {
			t.Fatalf("Report %s failed with status %d", rt, wRep.Code)
		}
	}

	// Test PDF Export
	reqPDF, _ := http.NewRequest("GET", "/api/v1/admin/reports?report_type=published_paper&export=pdf", nil)
	reqPDF.Header.Set("Authorization", "Bearer "+adminToken)
	wPDF := httptest.NewRecorder()
	r.ServeHTTP(wPDF, reqPDF)
	if wPDF.Code != http.StatusOK || !bytes.HasPrefix(wPDF.Body.Bytes(), []byte("%PDF-1.4")) {
		t.Fatalf("Expected valid PDF export from report generator")
	}

	// Test Excel CSV Export
	reqCSV, _ := http.NewRequest("GET", "/api/v1/admin/reports?report_type=published_paper&export=excel", nil)
	reqCSV.Header.Set("Authorization", "Bearer "+adminToken)
	wCSV := httptest.NewRecorder()
	r.ServeHTTP(wCSV, reqCSV)
	if wCSV.Code != http.StatusOK || wCSV.Body.Len() < 10 {
		t.Fatalf("Expected valid Excel CSV export from report generator")
	}
}
