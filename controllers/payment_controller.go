package controllers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"researchhub-backend/config"
	"researchhub-backend/models"
	"researchhub-backend/services"
	"researchhub-backend/utils"

	"github.com/gin-gonic/gin"
)

// CreatePaymentOrder creates a Razorpay order for the ₹1000 research paper submission fee
func CreatePaymentOrder(c *gin.Context) {
	studentID := c.GetString("user_id")
	studentName := c.GetString("user_name")
	studentEmail := c.GetString("user_email")

	var req struct {
		PaperTitle string `json:"paper_title"`
	}
	_ = c.ShouldBindJSON(&req)

	feeINR := 1000
	if config.Cfg != nil && config.Cfg.SubmissionFeeINR > 0 {
		feeINR = config.Cfg.SubmissionFeeINR
	}
	amountPaise := feeINR * 100
	receiptNo := fmt.Sprintf("RHBILL-%d-%s", time.Now().Year(), strings.ToUpper(utils.GenerateSecureToken(4)))

	keyID := ""
	keySecret := ""
	if config.Cfg != nil {
		keyID = config.Cfg.RazorpayKeyID
		keySecret = config.Cfg.RazorpayKeySecret
	}
	if keyID == "" || keySecret == "" {
		utils.RespondError(c, http.StatusInternalServerError, "Razorpay credentials are not configured", nil)
		return
	}

	orderPayload, _ := json.Marshal(map[string]interface{}{
		"amount":   amountPaise,
		"currency": "INR",
		"receipt":  receiptNo,
		"notes": map[string]string{
			"student_id":    studentID,
			"student_email": studentEmail,
			"paper_title":   req.PaperTitle,
		},
	})

	httpReq, err := http.NewRequest("POST", "https://api.razorpay.com/v1/orders", bytes.NewReader(orderPayload))
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "Failed to prepare Razorpay order request", err.Error())
		return
	}
	httpReq.SetBasicAuth(keyID, keySecret)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		utils.RespondError(c, http.StatusBadGateway, "Failed to connect to Razorpay gateway", err.Error())
		return
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		utils.RespondError(c, http.StatusBadGateway, "Razorpay order creation failed: "+string(respBytes), nil)
		return
	}

	var rzpOrder map[string]interface{}
	_ = json.Unmarshal(respBytes, &rzpOrder)
	orderID, _ := rzpOrder["id"].(string)

	payment := models.Payment{
		ReceiptNo:       receiptNo,
		StudentID:       studentID,
		StudentName:     studentName,
		StudentEmail:    studentEmail,
		PaperTitle:      strings.TrimSpace(req.PaperTitle),
		AmountINR:       feeINR,
		Currency:        "INR",
		RazorpayOrderID: orderID,
		Status:          "CREATED",
	}
	_ = config.DB.Create(&payment).Error

	utils.RespondSuccess(c, http.StatusCreated, "Razorpay order created", gin.H{
		"order_id":   orderID,
		"amount":     amountPaise,
		"amount_inr": feeINR,
		"currency":   "INR",
		"key_id":     keyID,
		"receipt_no": receiptNo,
		"payment_id": payment.ID,
	})
}

// VerifyPayment verifies the Razorpay signature, generates the ResearchHub PDF bill, and emails it to the student
func VerifyPayment(c *gin.Context) {
	studentID := c.GetString("user_id")

	var req struct {
		RazorpayOrderID   string `json:"razorpay_order_id" binding:"required"`
		RazorpayPaymentID string `json:"razorpay_payment_id" binding:"required"`
		RazorpaySignature string `json:"razorpay_signature" binding:"required"`
		PaperTitle        string `json:"paper_title"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "Payment verification fields are required", err.Error())
		return
	}

	keySecret := ""
	if config.Cfg != nil {
		keySecret = config.Cfg.RazorpayKeySecret
	}

	mac := hmac.New(sha256.New, []byte(keySecret))
	mac.Write([]byte(req.RazorpayOrderID + "|" + req.RazorpayPaymentID))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expectedSig), []byte(req.RazorpaySignature)) {
		utils.RespondError(c, http.StatusBadRequest, "Invalid Razorpay payment signature", nil)
		return
	}

	var payment models.Payment
	if err := config.DB.Where("razorpay_order_id = ? AND student_id = ?", req.RazorpayOrderID, studentID).First(&payment).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Payment order not found", nil)
		return
	}

	var user models.User
	_ = config.DB.Where("id = ?", studentID).First(&user).Error

	now := time.Now()
	storageDir := "./storage"
	if config.Cfg != nil && config.Cfg.StorageLocalPath != "" {
		storageDir = config.Cfg.StorageLocalPath
	}

	title := payment.PaperTitle
	if strings.TrimSpace(req.PaperTitle) != "" {
		title = strings.TrimSpace(req.PaperTitle)
	}

	billPath, _ := utils.GeneratePaymentBillPDF(
		storageDir,
		payment.ReceiptNo,
		user.FullName,
		user.Email,
		user.Institution,
		title,
		req.RazorpayPaymentID,
		req.RazorpayOrderID,
		now.Format("January 02, 2006 15:04"),
		payment.AmountINR,
	)

	_ = config.DB.Model(&payment).Updates(map[string]interface{}{
		"razorpay_payment_id": req.RazorpayPaymentID,
		"razorpay_signature":  req.RazorpaySignature,
		"paper_title":         title,
		"status":              "PAID",
		"bill_pdf_path":       billPath,
		"paid_at":             now,
	}).Error

	if user.Email != "" && billPath != "" {
		go services.SendEmailWithAttachmentAsync(
			user.Email,
			user.FullName,
			fmt.Sprintf("Payment Receipt (%s) - ResearchHub", payment.ReceiptNo),
			fmt.Sprintf("Thank you for your payment of <strong>Rs. %d.00 (INR)</strong> for research paper submission.<br/><br/>Receipt No: <strong>%s</strong><br/>Razorpay Payment ID: <strong>%s</strong><br/><br/>Your official ResearchHub Payment Bill PDF is attached to this email.", payment.AmountINR, payment.ReceiptNo, req.RazorpayPaymentID),
			billPath,
			payment.ReceiptNo+"_Bill.pdf",
		)
	}

	utils.RespondSuccess(c, http.StatusOK, "Payment verified and bill generated successfully", gin.H{
		"payment_id":          payment.ID,
		"receipt_no":          payment.ReceiptNo,
		"razorpay_payment_id": req.RazorpayPaymentID,
		"amount_inr":          payment.AmountINR,
		"status":              "PAID",
	})
}

// DownloadPaymentBill streams the generated PDF payment bill to the student
func DownloadPaymentBill(c *gin.Context) {
	studentID := c.GetString("user_id")
	receiptNo := c.Param("receiptNo")

	var payment models.Payment
	if err := config.DB.Where("receipt_no = ? AND student_id = ?", receiptNo, studentID).First(&payment).Error; err != nil {
		utils.RespondError(c, http.StatusNotFound, "Payment bill not found", nil)
		return
	}

	safePath, err := services.Storage.ResolveSafePath(payment.BillPDFPath)
	if err != nil {
		utils.RespondError(c, http.StatusNotFound, "Bill PDF file unavailable", nil)
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_Bill.pdf\"", payment.ReceiptNo))
	c.Header("Content-Type", "application/pdf")
	c.File(safePath)
}
