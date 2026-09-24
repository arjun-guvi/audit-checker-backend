package worker

import (
	"auditApp/config"
	"auditApp/models"
	"bytes"
	"context"
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SAPWorker handles periodic SAP checks and email reminders
type SAPWorker struct {
	redisPool    interface{} // Keep for future use
	queueName    string
	ticker       *time.Ticker
	stopChan     chan struct{}
	smtpHost     string
	smtpPort     string
	smtpUsername string
	smtpPassword string
	smtpFrom     string
}

// NewSAPWorker creates a new SAP worker instance
func NewSAPWorker() *SAPWorker {
	return &SAPWorker{
		stopChan: make(chan struct{}),
	}
}

// Start begins the SAP worker with 30 minute interval checks
func (w *SAPWorker) Start() error {
	// Load SMTP configuration from environment
	w.smtpHost = config.GetEnv("SMTP_HOST", "smtp.gmail.com")
	w.smtpPort = config.GetEnv("SMTP_PORT", "587")
	w.smtpUsername = config.GetEnv("SMTP_USERNAME", "")
	w.smtpPassword = config.GetEnv("SMTP_PASSWORD", "")
	w.smtpFrom = config.GetEnv("SMTP_FROM", "")

	// Start ticker for 30 minutes
	w.ticker = time.NewTicker(30 * time.Minute)

	log.Println("SAP Worker started - checking every 30 minutes")

	// Run first check immediately
	w.checkSAPAndSendReminders()

	// Start the loop
	for {
		select {
		case <-w.ticker.C:
			log.Println("SAP Worker: Running scheduled check at", time.Now().Format(time.RFC3339))
			w.checkSAPAndSendReminders()
		case <-w.stopChan:
			log.Println("SAP Worker stopped")
			return nil
		}
	}
}

// Stop stops the SAP worker
func (w *SAPWorker) Stop() {
	if w.ticker != nil {
		w.ticker.Stop()
	}
	close(w.stopChan)
}

// checkSAPAndSendReminders checks SAP records and sends reminders
func (w *SAPWorker) checkSAPAndSendReminders() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collection := config.MongoDB.Collection(models.SAPCollection())

	// Find active SAP records that need reminders
	// Criteria: IsActive=true, and (NextReminderDue <= now OR LastReminderSent is null)
	filter := bson.M{
		"is_active": true,
		"$or": []bson.M{
			{"next_reminder_due": bson.M{"$lte": time.Now()}},
			{"last_reminder_sent": nil},
		},
	}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		log.Printf("SAP Worker: Error fetching SAP records: %v", err)
		return
	}
	defer cursor.Close(ctx)

	var sapRecords []models.SAP
	if err = cursor.All(ctx, &sapRecords); err != nil {
		log.Printf("SAP Worker: Error parsing SAP records: %v", err)
		return
	}

	log.Printf("SAP Worker: Found %d records requiring reminders", len(sapRecords))

	// Process each SAP record
	for _, sap := range sapRecords {
		w.processSAPRecord(sap)
	}
}

// processSAPRecord processes a single SAP record and sends reminder
func (w *SAPWorker) processSAPRecord(sap models.SAP) {
	// Get recipients
	recipients := sap.MailsToSendTo
	if len(recipients) == 0 {
		log.Printf("SAP Worker: No recipients configured for SAP ID %s", sap.ID)
		return
	}

	// Send email reminder
	err := w.SendReminderEmail(sap, recipients)
	if err != nil {
		log.Printf("SAP Worker: Failed to send email for SAP ID %s: %v", sap.ID, err)
		return
	}

	// Update SAP record with reminder sent time
	w.updateSAPReminderStatus(sap.ID)
}

// SendReminderEmail sends a reminder email for SAP record (exported for controller use)
func (w *SAPWorker) SendReminderEmail(sap models.SAP, recipients []string) error {
	if w.smtpUsername == "" || w.smtpPassword == "" {
		return fmt.Errorf("SMTP credentials not configured")
	}

	subject := fmt.Sprintf("SAP Reminder - %s (Zen ID: %s)", sap.StudentFullName, sap.ZenID)
	body := w.generateEmailBody(sap)

	message := fmt.Sprintf("From: %s\r\n", w.smtpFrom)
	message += fmt.Sprintf("To: %s\r\n", strings.Join(recipients, ","))
	message += fmt.Sprintf("Subject: %s\r\n", subject)
	message += "MIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n\r\n"
	message += body

	auth := smtp.PlainAuth("", w.smtpUsername, w.smtpPassword, w.smtpHost)

	err := smtp.SendMail(
		fmt.Sprintf("%s:%s", w.smtpHost, w.smtpPort),
		auth,
		w.smtpFrom,
		recipients,
		[]byte(message),
	)

	if err != nil {
		return fmt.Errorf("failed to send email: %v", err)
	}

	log.Printf("SAP Worker: Successfully sent reminder for SAP ID %s to %v", sap.ID, recipients)
	return nil
}

// generateEmailBody generates the HTML email body
func (w *SAPWorker) generateEmailBody(sap models.SAP) string {
	var body bytes.Buffer

	body.WriteString("<!DOCTYPE html>")
	body.WriteString("<html>")
	body.WriteString("<head><title>SAP Reminder</title></head>")
	body.WriteString("<body style=\"font-family: Arial, sans-serif; padding: 20px; background-color: #f5f5f5;\">")

	body.WriteString("<div style=\"max-width: 700px; margin: 0 auto; background: white; padding: 30px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1);\">")

	// Header
	body.WriteString("<div style=\"background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 20px; border-radius: 8px; margin-bottom: 20px;\">")
	body.WriteString(fmt.Sprintf("<h2 style=\"margin: 0;\">SAP Reminder - %s</h2>", sap.StudentFullName))
	body.WriteString(fmt.Sprintf("<p style=\"margin: 5px 0 0 0;\">Zen ID: %s | Reminder #%d</p>", sap.ZenID, sap.ReminderCount+1))
	body.WriteString("</div>")

	// Student Details
	body.WriteString("<div style=\"background: #f8f9fa; padding: 20px; border-radius: 8px; margin-bottom: 20px;\">")
	body.WriteString("<h3 style=\"color: #333; margin-top: 0;\">Student Details</h3>")
	body.WriteString("<table style=\"width: 100%; border-collapse: collapse;\">")

	details := [][]string{
		{"Email", sap.Email},
		{"Phone", sap.PrimaryPhone},
		{"Course", sap.Course},
		{"Stage", sap.Stage},
		{"Payment Status", sap.PaymentStatus},
		{"Total Paid", sap.TotalPaid},
		{"Balance Amount", sap.BalanceAmount},
		{"EMI Status", sap.EMIStatus},
		{"Assigned Batch", sap.AssignedBatch},
		{"Preferred Language", sap.PreferredLanguage},
		{"Sale Owner", sap.SaleOwner},
		{"Last Modified", sap.ModifiedTime},
	}

	for _, detail := range details {
		if detail[1] != "" {
			body.WriteString(fmt.Sprintf("<tr><td style=\"padding: 8px; font-weight: bold; width: 40%%; color: #666;\">%s</td>", detail[0]))
			body.WriteString(fmt.Sprintf("<td style=\"padding: 8px; color: #333;\">%s</td></tr>", detail[1]))
		}
	}
	body.WriteString("</table></div>")

	// Payment Information
	if sap.BalanceAmount != "" && sap.BalanceAmount != "0.00" {
		body.WriteString("<div style=\"background: #fff3cd; padding: 20px; border-radius: 8px; margin-bottom: 20px; border-left: 4px solid #ffc107;\">")
		body.WriteString("<h3 style=\"color: #856404; margin-top: 0;\">⚠️ Payment Pending</h3>")
		body.WriteString(fmt.Sprintf("<p><strong>Balance Amount:</strong> ₹%s</p>", sap.BalanceAmount))
		if sap.PaymentType != "" {
			body.WriteString(fmt.Sprintf("<p><strong>Payment Type:</strong> %s</p>", sap.PaymentType))
		}
		body.WriteString("</div>")
	}

	// Call to Action
	body.WriteString("<div style=\"text-align: center; margin-top: 20px;\">")
	body.WriteString("<a href=\"https://zenclass.in\" style=\"background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 12px 30px; text-decoration: none; border-radius: 25px; display: inline-block; font-weight: bold;\">View Details</a>")
	body.WriteString("</div>")

	// Footer
	body.WriteString("<div style=\"margin-top: 30px; border-top: 1px solid #eee; padding-top: 20px; text-align: center; color: #888; font-size: 12px;\">")
	body.WriteString("<p>This is an automated reminder from GUVI-ZEN SAP System.</p>")
	body.WriteString(fmt.Sprintf("<p>Reminder sent on: %s</p>", time.Now().Format("02-Jan-2006 15:04:05")))
	body.WriteString("</div>")

	body.WriteString("</div>")
	body.WriteString("</body></html>")

	return body.String()
}

// updateSAPReminderStatus updates the SAP record after sending reminder
func (w *SAPWorker) updateSAPReminderStatus(sapID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := config.MongoDB.Collection(models.SAPCollection())

	now := time.Now()
	nextReminder := now.Add(30 * time.Minute)

	update := bson.M{
		"$set": bson.M{
			"last_reminder_sent": now,
			"next_reminder_due":  nextReminder,
			"updated_at":         now,
		},
		"$inc": bson.M{
			"reminder_count": 1,
		},
	}

	_, err := collection.UpdateByID(ctx, bson.M{"_id": sapID}, update)
	if err != nil {
		log.Printf("SAP Worker: Failed to update SAP record %s: %v", sapID, err)
	} else {
		log.Printf("SAP Worker: Updated reminder status for SAP ID %s", sapID)
	}
}

// AddRecipientToSAP adds email recipients to a SAP record
func AddRecipientToSAP(sapID string, emails []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := config.MongoDB.Collection(models.SAPCollection())

	update := bson.M{
		"$addToSet": bson.M{
			"mails_to_send_to": bson.M{"$each": emails},
		},
		"$set": bson.M{
			"updated_at": time.Now(),
		},
	}

	result, err := collection.UpdateByID(ctx, sapID, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("SAP record not found")
	}

	return nil
}

// GetSAPRecordsNeedingReminders returns SAP records that need reminders
func GetSAPRecordsNeedingReminders(limit int64) ([]models.SAP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collection := config.MongoDB.Collection(models.SAPCollection())

	filter := bson.M{
		"is_active": true,
		"$or": []bson.M{
			{"next_reminder_due": bson.M{"$lte": time.Now()}},
			{"last_reminder_sent": nil},
		},
	}

	opts := options.Find().SetLimit(limit).SetSort(bson.D{{Key: "next_reminder_due", Value: 1}})
	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var records []models.SAP
	if err = cursor.All(ctx, &records); err != nil {
		return nil, err
	}

	return records, nil
}

// RunSAPReminderCheck is a utility function to manually trigger reminder check
func RunSAPReminderCheck() {
	worker := NewSAPWorker()
	worker.checkSAPAndSendReminders()
}
