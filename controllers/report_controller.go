package controllers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"researchhub-backend/config"
	"researchhub-backend/models"
	"researchhub-backend/utils"

	"github.com/gin-gonic/gin"
)

// GenerateAdminReport serves all 13 required Super Admin reports from actual database records with screen view, pagination, summary cards, charts, PDF export, and Excel export
func GenerateAdminReport(c *gin.Context) {
	reportType := strings.ToLower(strings.TrimSpace(c.DefaultQuery("report_type", "paper_submission")))
	startDateStr := strings.TrimSpace(c.Query("start_date"))
	endDateStr := strings.TrimSpace(c.Query("end_date"))
	pubYearStr := strings.TrimSpace(c.Query("publication_year"))
	paperStatus := strings.TrimSpace(c.Query("paper_status"))
	categoryID := strings.TrimSpace(c.Query("category_id"))
	studentID := strings.TrimSpace(c.Query("student_id"))
	reviewerID := strings.TrimSpace(c.Query("reviewer_id"))
	institution := strings.TrimSpace(c.Query("institution"))
	search := strings.TrimSpace(c.Query("search"))
	exportFormat := strings.ToLower(strings.TrimSpace(c.Query("export"))) // "", "pdf", "excel"
	page, limit, offset := utils.GetPaginationParams(c)

	var startDate, endDate *time.Time
	if startDateStr != "" {
		if t, err := time.Parse("2006-01-02", startDateStr); err == nil {
			startDate = &t
		}
	}
	if endDateStr != "" {
		if t, err := time.Parse("2006-01-02", endDateStr); err == nil {
			tEnd := t.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
			endDate = &tEnd
		}
	}

	reportTitle := "Research Paper Submission Report"
	var headers []string
	var allRows [][]string
	var chartLabels []string
	var chartValues []float64
	summaryCards := []gin.H{}

	switch reportType {
	case "1", "student_registration":
		reportTitle = "1. Student Registration Report"
		headers = []string{"Student Name", "Email", "Institution", "Department", "Degree Program", "Status", "Registered Date"}
		q := config.DB.Model(&models.User{}).Preload("StudentProfile").Where("role_name = ?", models.RoleStudent)
		if startDate != nil {
			q = q.Where("created_at >= ?", *startDate)
		}
		if endDate != nil {
			q = q.Where("created_at <= ?", *endDate)
		}
		if institution != "" {
			q = q.Where("LOWER(institution) LIKE ?", "%"+strings.ToLower(institution)+"%")
		}
		if studentID != "" {
			q = q.Where("id = ?", studentID)
		}
		if search != "" {
			like := "%" + strings.ToLower(search) + "%"
			q = q.Where("LOWER(full_name) LIKE ? OR LOWER(email) LIKE ? OR LOWER(institution) LIKE ?", like, like, like)
		}
		var students []models.User
		q.Order("created_at DESC").Find(&students)
		instCounts := map[string]int{}
		activeCnt := 0
		for _, s := range students {
			deg := s.Designation
			if s.StudentProfile != nil && s.StudentProfile.DegreeProgram != "" {
				deg = s.StudentProfile.DegreeProgram
			}
			st := "Active"
			if !s.IsActive {
				st = "Inactive"
			} else {
				activeCnt++
			}
			instCounts[s.Institution]++
			allRows = append(allRows, []string{
				s.FullName, s.Email, s.Institution, s.Department, deg, st, s.CreatedAt.Format("2006-01-02"),
			})
		}
		for k, v := range instCounts {
			chartLabels = append(chartLabels, k)
			chartValues = append(chartValues, float64(v))
		}
		summaryCards = []gin.H{
			{"label": "Total Registered Students", "value": len(students)},
			{"label": "Active Accounts", "value": activeCnt},
			{"label": "Institutions Represented", "value": len(instCounts)},
		}

	case "2", "paper_submission":
		reportTitle = "2. Research Paper Submission Report"
		headers = []string{"Submission ID", "Title", "Author", "Institution", "Category", "Status", "Submitted Date"}
		papers := queryFilteredPapers(startDate, endDate, paperStatus, categoryID, studentID, institution, search)
		statusCounts := map[string]int{}
		for _, p := range papers {
			catName := ""
			if p.Category != nil {
				catName = p.Category.Name
			}
			statusCounts[p.Status]++
			allRows = append(allRows, []string{
				p.SubmissionCode, p.Title, p.CorrespondingAuthor, p.Institution, catName, p.Status, p.SubmittedAt.Format("2006-01-02"),
			})
		}
		for k, v := range statusCounts {
			chartLabels = append(chartLabels, k)
			chartValues = append(chartValues, float64(v))
		}
		summaryCards = []gin.H{
			{"label": "Total Submissions", "value": len(papers)},
			{"label": "Distinct Statuses", "value": len(statusCounts)},
		}

	case "3", "pending_review":
		reportTitle = "3. Pending Review Report"
		headers = []string{"Submission ID", "Paper Title", "Reviewer", "Reviewer Email", "Round", "Assignment Status", "Due Date"}
		q := config.DB.Model(&models.PaperAssignment{}).Preload("Paper").Preload("Reviewer").Where("status IN ?", []string{"ASSIGNED", "IN_PROGRESS"})
		if reviewerID != "" {
			q = q.Where("reviewer_id = ?", reviewerID)
		}
		if startDate != nil {
			q = q.Where("created_at >= ?", *startDate)
		}
		if endDate != nil {
			q = q.Where("created_at <= ?", *endDate)
		}
		var assignments []models.PaperAssignment
		q.Order("due_date ASC").Find(&assignments)
		revCounts := map[string]int{}
		for _, a := range assignments {
			code, title := "", ""
			if a.Paper != nil {
				code = a.Paper.SubmissionCode
				title = a.Paper.Title
			}
			revName, revEmail := "", ""
			if a.Reviewer != nil {
				revName = a.Reviewer.FullName
				revEmail = a.Reviewer.Email
			}
			revCounts[revName]++
			allRows = append(allRows, []string{
				code, title, revName, revEmail, fmt.Sprint(a.ReviewRound), a.Status, a.DueDate.Format("2006-01-02"),
			})
		}
		for k, v := range revCounts {
			chartLabels = append(chartLabels, k)
			chartValues = append(chartValues, float64(v))
		}
		summaryCards = []gin.H{
			{"label": "Pending Review Assignments", "value": len(assignments)},
			{"label": "Reviewers With Active Queue", "value": len(revCounts)},
		}

	case "4", "reviewer_performance":
		reportTitle = "4. Reviewer Performance Report"
		headers = []string{"Reviewer Name", "Email", "Institution", "Assigned", "Completed", "Pending", "Avg Score Given"}
		q := config.DB.Model(&models.User{}).Where("role_name = ?", models.RoleSuperAdmin)
		if reviewerID != "" {
			q = q.Where("id = ?", reviewerID)
		}
		if institution != "" {
			q = q.Where("LOWER(institution) LIKE ?", "%"+strings.ToLower(institution)+"%")
		}
		var reviewers []models.User
		q.Order("full_name ASC").Find(&reviewers)
		var totalCompleted int64
		for _, r := range reviewers {
			var assigned, completed, pending int64
			var avgScore float64
			config.DB.Model(&models.PaperAssignment{}).Where("reviewer_id = ?", r.ID).Count(&assigned)
			config.DB.Model(&models.PaperAssignment{}).Where("reviewer_id = ? AND status = ?", r.ID, "COMPLETED").Count(&completed)
			config.DB.Model(&models.PaperAssignment{}).Where("reviewer_id = ? AND status != ?", r.ID, "COMPLETED").Count(&pending)
			config.DB.Model(&models.PaperReview{}).Where("reviewer_id = ?", r.ID).Select("COALESCE(AVG(overall_score), 0)").Scan(&avgScore)
			totalCompleted += completed
			chartLabels = append(chartLabels, r.FullName)
			chartValues = append(chartValues, float64(completed))
			allRows = append(allRows, []string{
				r.FullName, r.Email, r.Institution, fmt.Sprint(assigned), fmt.Sprint(completed), fmt.Sprint(pending), fmt.Sprintf("%.1f / 10", avgScore),
			})
		}
		summaryCards = []gin.H{
			{"label": "Active Peer Reviewers", "value": len(reviewers)},
			{"label": "Total Reviews Completed", "value": totalCompleted},
		}

	case "5", "revision":
		reportTitle = "5. Revision Report"
		headers = []string{"Submission ID", "Paper Title", "Author", "Round", "Revision Status", "Editorial Instructions", "Requested Date"}
		var revisions []models.RevisionHistory
		config.DB.Order("requested_at DESC").Find(&revisions)
		pendingCnt, submittedCnt := 0, 0
		for _, rev := range revisions {
			var p models.ResearchPaper
			if err := config.DB.Where("id = ?", rev.PaperID).First(&p).Error; err != nil {
				continue
			}
			if studentID != "" && p.StudentID != studentID {
				continue
			}
			if categoryID != "" && p.CategoryID != categoryID {
				continue
			}
			if rev.Status == "SUBMITTED" {
				submittedCnt++
			} else {
				pendingCnt++
			}
			allRows = append(allRows, []string{
				p.SubmissionCode, p.Title, p.CorrespondingAuthor, fmt.Sprint(rev.RevisionRound), rev.Status, rev.RequestComments, rev.RequestedAt.Format("2006-01-02"),
			})
		}
		chartLabels = []string{"Pending Revisions", "Submitted Revisions"}
		chartValues = []float64{float64(pendingCnt), float64(submittedCnt)}
		summaryCards = []gin.H{
			{"label": "Total Revision Requests", "value": len(allRows)},
			{"label": "Awaiting Author Upload", "value": pendingCnt},
			{"label": "Revisions Resubmitted", "value": submittedCnt},
		}

	case "6", "approved_paper":
		reportTitle = "6. Approved Paper Report"
		headers = []string{"Submission ID", "Title", "Author", "Institution", "Category", "Status", "Approved Date"}
		papers := queryFilteredPapers(startDate, endDate, models.StatusApproved, categoryID, studentID, institution, search)
		readyPapers := queryFilteredPapers(startDate, endDate, models.StatusReadyForPublication, categoryID, studentID, institution, search)
		pubPapers := queryFilteredPapers(startDate, endDate, models.StatusPublished, categoryID, studentID, institution, search)
		combined := append(append(papers, readyPapers...), pubPapers...)
		for _, p := range combined {
			catName := ""
			if p.Category != nil {
				catName = p.Category.Name
			}
			appDate := p.UpdatedAt.Format("2006-01-02")
			if p.ApprovedAt != nil {
				appDate = p.ApprovedAt.Format("2006-01-02")
			}
			allRows = append(allRows, []string{
				p.SubmissionCode, p.Title, p.CorrespondingAuthor, p.Institution, catName, p.Status, appDate,
			})
		}
		chartLabels = []string{"Approved", "Ready for Publication", "Published"}
		chartValues = []float64{float64(len(papers)), float64(len(readyPapers)), float64(len(pubPapers))}
		summaryCards = []gin.H{
			{"label": "Total Approved Manuscripts", "value": len(combined)},
		}

	case "7", "rejected_paper":
		reportTitle = "7. Rejected Paper Report"
		headers = []string{"Submission ID", "Title", "Author", "Institution", "Category", "Editorial Notes", "Decision Date"}
		papers := queryFilteredPapers(startDate, endDate, models.StatusRejected, categoryID, studentID, institution, search)
		for _, p := range papers {
			catName := ""
			if p.Category != nil {
				catName = p.Category.Name
			}
			allRows = append(allRows, []string{
				p.SubmissionCode, p.Title, p.CorrespondingAuthor, p.Institution, catName, p.EditorialNotes, p.UpdatedAt.Format("2006-01-02"),
			})
		}
		chartLabels = []string{"Rejected Manuscripts"}
		chartValues = []float64{float64(len(papers))}
		summaryCards = []gin.H{
			{"label": "Total Rejected Papers", "value": len(papers)},
		}

	case "8", "published_paper":
		reportTitle = "8. Published Paper Report"
		headers = []string{"Publication ID", "Paper ID", "Title", "Author", "Journal", "Vol/Issue", "Year", "Downloads"}
		q := config.DB.Model(&models.Publication{}).Preload("Paper.Category").Where("is_published = ?", true)
		if pubYearStr != "" {
			if yr, err := strconv.Atoi(pubYearStr); err == nil {
				q = q.Where("publication_year = ?", yr)
			}
		}
		if startDate != nil {
			q = q.Where("publication_date >= ?", *startDate)
		}
		if endDate != nil {
			q = q.Where("publication_date <= ?", *endDate)
		}
		var pubs []models.Publication
		q.Order("publication_date DESC").Find(&pubs)
		totalDownloads := 0
		for _, pub := range pubs {
			subCode, authorName := "", ""
			if pub.Paper != nil {
				subCode = pub.Paper.SubmissionCode
				authorName = pub.Paper.CorrespondingAuthor
				if categoryID != "" && pub.Paper.CategoryID != categoryID {
					continue
				}
				if institution != "" && !strings.Contains(strings.ToLower(pub.Paper.Institution), strings.ToLower(institution)) {
					continue
				}
			}
			totalDownloads += pub.DownloadCount
			chartLabels = append(chartLabels, pub.PublicationCode)
			chartValues = append(chartValues, float64(pub.DownloadCount))
			allRows = append(allRows, []string{
				pub.PublicationCode, subCode, pub.PublicationTitle, authorName, pub.JournalName, fmt.Sprintf("Vol %s, Iss %s", pub.Volume, pub.Issue), fmt.Sprint(pub.PublicationYear), fmt.Sprint(pub.DownloadCount),
			})
		}
		summaryCards = []gin.H{
			{"label": "Total Published Papers", "value": len(allRows)},
			{"label": "Total PDF Downloads", "value": totalDownloads},
		}

	case "9", "certificate_generation":
		reportTitle = "9. Certificate Generation Report"
		headers = []string{"Certificate ID", "Author Name", "Paper Title", "Publication ID", "Vol/Issue", "Status", "Issued Date"}
		q := config.DB.Model(&models.Certificate{})
		if studentID != "" {
			q = q.Where("student_id = ?", studentID)
		}
		if startDate != nil {
			q = q.Where("issued_at >= ?", *startDate)
		}
		if endDate != nil {
			q = q.Where("issued_at <= ?", *endDate)
		}
		var certs []models.Certificate
		q.Order("issued_at DESC").Find(&certs)
		validCnt, revokedCnt := 0, 0
		for _, cert := range certs {
			st := "VALID"
			if cert.IsRevoked {
				st = "REVOKED"
				revokedCnt++
			} else {
				validCnt++
			}
			allRows = append(allRows, []string{
				cert.CertificateCode, cert.AuthorName, cert.PaperTitle, cert.PublicationCode, fmt.Sprintf("Vol %s, Iss %s", cert.Volume, cert.Issue), st, cert.IssuedAt.Format("2006-01-02"),
			})
		}
		chartLabels = []string{"Valid Certificates", "Revoked Certificates"}
		chartValues = []float64{float64(validCnt), float64(revokedCnt)}
		summaryCards = []gin.H{
			{"label": "Total Certificates Issued", "value": len(certs)},
			{"label": "Active / Valid", "value": validCnt},
		}

	case "10", "monthly_publication":
		reportTitle = "10. Monthly Publication Report"
		headers = []string{"Month & Year", "Published Papers Count", "Total Downloads", "Total Views"}
		var pubs []models.Publication
		q := config.DB.Model(&models.Publication{}).Where("is_published = ?", true)
		if pubYearStr != "" {
			if yr, err := strconv.Atoi(pubYearStr); err == nil {
				q = q.Where("publication_year = ?", yr)
			}
		}
		q.Order("publication_date DESC").Find(&pubs)
		type mAgg struct {
			count     int
			downloads int
			views     int
		}
		mMap := map[string]*mAgg{}
		var orderKeys []string
		for _, p := range pubs {
			k := p.PublicationDate.Format("January 2006")
			if _, ok := mMap[k]; !ok {
				mMap[k] = &mAgg{}
				orderKeys = append(orderKeys, k)
			}
			mMap[k].count++
			mMap[k].downloads += p.DownloadCount
			mMap[k].views += p.ViewCount
		}
		for _, k := range orderKeys {
			agg := mMap[k]
			chartLabels = append(chartLabels, k)
			chartValues = append(chartValues, float64(agg.count))
			allRows = append(allRows, []string{
				k, fmt.Sprint(agg.count), fmt.Sprint(agg.downloads), fmt.Sprint(agg.views),
			})
		}
		summaryCards = []gin.H{
			{"label": "Active Publication Months", "value": len(orderKeys)},
			{"label": "Total Published Papers", "value": len(pubs)},
		}

	case "11", "yearly_publication":
		reportTitle = "11. Yearly Publication Report"
		headers = []string{"Publication Year", "Published Papers", "Total Downloads", "Total Views"}
		var pubs []models.Publication
		config.DB.Model(&models.Publication{}).Where("is_published = ?", true).Order("publication_year DESC").Find(&pubs)
		type yAgg struct {
			count     int
			downloads int
			views     int
		}
		yMap := map[int]*yAgg{}
		var years []int
		for _, p := range pubs {
			if _, ok := yMap[p.PublicationYear]; !ok {
				yMap[p.PublicationYear] = &yAgg{}
				years = append(years, p.PublicationYear)
			}
			yMap[p.PublicationYear].count++
			yMap[p.PublicationYear].downloads += p.DownloadCount
			yMap[p.PublicationYear].views += p.ViewCount
		}
		for _, yr := range years {
			agg := yMap[yr]
			chartLabels = append(chartLabels, fmt.Sprint(yr))
			chartValues = append(chartValues, float64(agg.count))
			allRows = append(allRows, []string{
				fmt.Sprint(yr), fmt.Sprint(agg.count), fmt.Sprint(agg.downloads), fmt.Sprint(agg.views),
			})
		}
		summaryCards = []gin.H{
			{"label": "Publication Years Recorded", "value": len(years)},
			{"label": "Cumulative Publications", "value": len(pubs)},
		}

	case "12", "category_wise":
		reportTitle = "12. Category-wise Research Report"
		headers = []string{"Category Code", "Category Name", "Discipline", "Total Submissions", "Under Review", "Approved / Published"}
		var categories []models.ResearchCategory
		config.DB.Order("name ASC").Find(&categories)
		for _, cat := range categories {
			if categoryID != "" && cat.ID != categoryID {
				continue
			}
			var total, reviewing, pubCnt int64
			config.DB.Model(&models.ResearchPaper{}).Where("category_id = ?", cat.ID).Count(&total)
			config.DB.Model(&models.ResearchPaper{}).Where("category_id = ? AND status IN ?", cat.ID, []string{
				models.StatusSubmitted, models.StatusInitialScreening, models.StatusUnderReview, models.StatusRevisionRequired, models.StatusRevisionSubmitted,
			}).Count(&reviewing)
			config.DB.Model(&models.ResearchPaper{}).Where("category_id = ? AND status IN ?", cat.ID, []string{
				models.StatusApproved, models.StatusReadyForPublication, models.StatusPublished,
			}).Count(&pubCnt)

			chartLabels = append(chartLabels, cat.Code)
			chartValues = append(chartValues, float64(total))
			allRows = append(allRows, []string{
				cat.Code, cat.Name, cat.Discipline, fmt.Sprint(total), fmt.Sprint(reviewing), fmt.Sprint(pubCnt),
			})
		}
		summaryCards = []gin.H{
			{"label": "Total Research Categories", "value": len(allRows)},
		}

	case "13", "institution_wise":
		reportTitle = "13. Institution-wise Research Report"
		headers = []string{"Institution / University", "Total Submissions", "Published Papers", "Approved Papers", "Under Review"}
		var papers []models.ResearchPaper
		config.DB.Find(&papers)
		type instAgg struct {
			total     int
			published int
			approved  int
			reviewing int
		}
		instMap := map[string]*instAgg{}
		var instNames []string
		for _, p := range papers {
			instName := strings.TrimSpace(p.Institution)
			if instName == "" {
				instName = "Unspecified Institution"
			}
			if institution != "" && !strings.Contains(strings.ToLower(instName), strings.ToLower(institution)) {
				continue
			}
			if _, ok := instMap[instName]; !ok {
				instMap[instName] = &instAgg{}
				instNames = append(instNames, instName)
			}
			instMap[instName].total++
			switch p.Status {
			case models.StatusPublished:
				instMap[instName].published++
			case models.StatusApproved, models.StatusReadyForPublication:
				instMap[instName].approved++
			default:
				instMap[instName].reviewing++
			}
		}
		for _, name := range instNames {
			agg := instMap[name]
			chartLabels = append(chartLabels, name)
			chartValues = append(chartValues, float64(agg.total))
			allRows = append(allRows, []string{
				name, fmt.Sprint(agg.total), fmt.Sprint(agg.published), fmt.Sprint(agg.approved), fmt.Sprint(agg.reviewing),
			})
		}
		summaryCards = []gin.H{
			{"label": "Total Participating Institutions", "value": len(instNames)},
			{"label": "Total Institutional Submissions", "value": len(papers)},
		}
	}

	// Check if export=pdf or export=excel requested
	if exportFormat == "pdf" {
		subtitle := fmt.Sprintf("Generated on %s  |  Total Records: %d", time.Now().Format("Jan 02, 2006 15:04"), len(allRows))
		pdfBytes := utils.GenerateReportPDF(reportTitle, subtitle, headers, allRows)
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.pdf\"", strings.ReplaceAll(reportType, " ", "_")))
		c.Header("Content-Type", "application/pdf")
		c.Data(http.StatusOK, "application/pdf", pdfBytes)
		return
	}

	if exportFormat == "excel" || exportFormat == "csv" {
		var buf bytes.Buffer
		// Write UTF-8 BOM so Microsoft Excel opens columns cleanly
		buf.Write([]byte{0xEF, 0xBB, 0xBF})
		writer := csv.NewWriter(&buf)
		_ = writer.Write(headers)
		for _, r := range allRows {
			_ = writer.Write(r)
		}
		writer.Flush()
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_Report.csv\"", reportType))
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
		return
	}

	// Paginate rows for on-screen view
	totalItems := int64(len(allRows))
	startIdx := offset
	if startIdx > len(allRows) {
		startIdx = len(allRows)
	}
	endIdx := startIdx + limit
	if endIdx > len(allRows) {
		endIdx = len(allRows)
	}
	pagedRows := allRows[startIdx:endIdx]

	totalPages := int((totalItems + int64(limit) - 1) / int64(limit))
	if totalPages < 1 {
		totalPages = 1
	}

	utils.RespondPaginated(c, "Report generated from database records", gin.H{
		"report_type":   reportType,
		"report_title":  reportTitle,
		"generated_at":  time.Now(),
		"summary_cards": summaryCards,
		"chart": gin.H{
			"labels": chartLabels,
			"values": chartValues,
		},
		"headers": headers,
		"rows":    pagedRows,
		"meta": gin.H{
			"page":        page,
			"limit":       limit,
			"total_items": totalItems,
			"total_pages": totalPages,
		},
	}, page, limit, totalItems)
}

func queryFilteredPapers(startDate, endDate *time.Time, status, categoryID, studentID, institution, search string) []models.ResearchPaper {
	q := config.DB.Model(&models.ResearchPaper{}).Preload("Category")
	if startDate != nil {
		q = q.Where("submitted_at >= ?", *startDate)
	}
	if endDate != nil {
		q = q.Where("submitted_at <= ?", *endDate)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if categoryID != "" {
		q = q.Where("category_id = ?", categoryID)
	}
	if studentID != "" {
		q = q.Where("student_id = ?", studentID)
	}
	if institution != "" {
		q = q.Where("LOWER(institution) LIKE ?", "%"+strings.ToLower(institution)+"%")
	}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		q = q.Where("LOWER(title) LIKE ? OR LOWER(submission_code) LIKE ? OR LOWER(corresponding_author) LIKE ?", like, like, like)
	}
	var papers []models.ResearchPaper
	q.Order("submitted_at DESC").Find(&papers)
	return papers
}
