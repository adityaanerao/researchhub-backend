package services

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"log"
	"net/smtp"
	"os"

	"researchhub-backend/config"
	"researchhub-backend/models"
)

func CreateNotification(userID, title, message, notifType, referenceID string) {
	if config.DB == nil || userID == "" {
		return
	}
	notif := models.Notification{
		UserID:      userID,
		Title:       title,
		Message:     message,
		Type:        notifType,
		ReferenceID: referenceID,
		IsRead:      false,
	}
	if err := config.DB.Create(&notif).Error; err != nil {
		log.Printf("[WARN] Failed to save in-app notification: %v", err)
	}

	var user models.User
	if err := config.DB.Select("email, full_name").Where("id = ?", userID).First(&user).Error; err == nil && user.Email != "" {
		go SendEmailAsync(user.Email, user.FullName, title, message)
	}
}

func SendEmailAsync(toEmail, recipientName, subject, bodyText string) {
	if config.Cfg == nil || !config.Cfg.SMTPEnabled || config.Cfg.SMTPHost == "" || config.Cfg.SMTPUsername == "" {
		return
	}

	addr := fmt.Sprintf("%s:%d", config.Cfg.SMTPHost, config.Cfg.SMTPPort)
	auth := smtp.PlainAuth("", config.Cfg.SMTPUsername, config.Cfg.SMTPPassword, config.Cfg.SMTPHost)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family: Arial, sans-serif; color: #1e293b; line-height: 1.6;">
  <div style="max-width: 600px; margin: 0 auto; border: 1px solid #e2e8f0; border-radius: 8px; overflow: hidden;">
    <div style="background: #1e293b; color: #ffffff; padding: 16px 24px;">
      <h2 style="margin: 0;">%s Editorial Office</h2>
    </div>
    <div style="padding: 24px;">
      <p>Dear <strong>%s</strong>,</p>
      <p>%s</p>
      <hr style="border: none; border-top: 1px solid #e2e8f0; margin: 20px 0;" />
      <p style="font-size: 12px; color: #64748b;">This is an automated academic notification from %s.</p>
    </div>
  </div>
</body>
</html>`, config.Cfg.AppName, recipientName, bodyText, config.Cfg.AppName)

	msg := []byte(fmt.Sprintf("From: %s <%s>\r\nTo: %s\r\nSubject: [%s] %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=\"UTF-8\"\r\n\r\n%s",
		config.Cfg.SMTPFromName, config.Cfg.SMTPFromEmail, toEmail, config.Cfg.AppName, subject, htmlBody))

	if err := smtp.SendMail(addr, auth, config.Cfg.SMTPFromEmail, []string{toEmail}, msg); err != nil {
		log.Printf("[WARN] SMTP email send failed for %s: %v", toEmail, err)
	}
}

// SendEmailWithAttachmentAsync sends an HTML email with a PDF attachment (e.g. Publication Certificate)
func SendEmailWithAttachmentAsync(toEmail, recipientName, subject, bodyText, attachmentPath, attachmentName string) {
	if config.Cfg == nil || !config.Cfg.SMTPEnabled || config.Cfg.SMTPHost == "" || config.Cfg.SMTPUsername == "" {
		return
	}

	fileBytes, err := os.ReadFile(attachmentPath)
	if err != nil {
		log.Printf("[WARN] Attachment read failed (%s), sending email without attachment: %v", attachmentPath, err)
		SendEmailAsync(toEmail, recipientName, subject, bodyText)
		return
	}

	addr := fmt.Sprintf("%s:%d", config.Cfg.SMTPHost, config.Cfg.SMTPPort)
	auth := smtp.PlainAuth("", config.Cfg.SMTPUsername, config.Cfg.SMTPPassword, config.Cfg.SMTPHost)
	boundary := "RESEARCHHUB_MIME_BOUNDARY_2026"

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family: Arial, sans-serif; color: #1e293b; line-height: 1.6;">
  <div style="max-width: 600px; margin: 0 auto; border: 1px solid #e2e8f0; border-radius: 8px; overflow: hidden;">
    <div style="background: #1e293b; color: #ffffff; padding: 16px 24px;">
      <h2 style="margin: 0;">%s Editorial Office</h2>
    </div>
    <div style="padding: 24px;">
      <p>Dear <strong>%s</strong>,</p>
      <p>%s</p>
      <hr style="border: none; border-top: 1px solid #e2e8f0; margin: 20px 0;" />
      <p style="font-size: 12px; color: #64748b;">Please find your official Publication Certificate PDF attached to this email.</p>
    </div>
  </div>
</body>
</html>`, config.Cfg.AppName, recipientName, bodyText)

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("From: %s <%s>\r\n", config.Cfg.SMTPFromName, config.Cfg.SMTPFromEmail))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", toEmail))
	buf.WriteString(fmt.Sprintf("Subject: [%s] %s\r\n", config.Cfg.AppName, subject))
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundary))

	// HTML Body Part
	buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	buf.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n")
	buf.WriteString(htmlBody + "\r\n\r\n")

	// PDF Attachment Part
	buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	buf.WriteString(fmt.Sprintf("Content-Type: application/pdf; name=\"%s\"\r\n", attachmentName))
	buf.WriteString("Content-Transfer-Encoding: base64\r\n")
	buf.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n\r\n", attachmentName))
	encoded := base64.StdEncoding.EncodeToString(fileBytes)
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		buf.WriteString(encoded[i:end] + "\r\n")
	}
	buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	if err := smtp.SendMail(addr, auth, config.Cfg.SMTPFromEmail, []string{toEmail}, buf.Bytes()); err != nil {
		log.Printf("[WARN] SMTP email with attachment failed for %s: %v", toEmail, err)
	}
}

func LogAudit(userID, userEmail, userRole, action, entityType, entityID, details, ipAddress string) {
	if config.DB == nil {
		return
	}
	entry := models.AuditLog{
		UserID:     userID,
		UserEmail:  userEmail,
		UserRole:   userRole,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Details:    details,
		IPAddress:  ipAddress,
	}
	_ = config.DB.Create(&entry).Error
}
