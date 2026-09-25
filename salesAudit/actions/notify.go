package actions

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/smtp"
	"strings"

	"auditApp/config"
	"auditApp/salesAudit/core"
	"auditApp/salesAudit/models"
	"auditApp/salesAudit/store"
)

// Timeline events, in-app notifications and mails.

// EnqueueMail hands a logged mail to the worker's send-mail job. The worker package sets it up;
// tests replace it.
var EnqueueMail = func(program, alertID string) error {
	return errors.New("the mail queue is not set up")
}

// SendSMTP delivers one HTML mail. Tests replace it.
var SendSMTP = func(to []string, subject, htmlBody string) error {
	message := "From: " + config.SMTPFrom + "\r\n" +
		"To: " + strings.Join(to, ",") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n\r\n" +
		htmlBody
	auth := smtp.PlainAuth("", config.SMTPUsername, config.SMTPPassword, config.SMTPHost)
	return smtp.SendMail(config.SMTPHost+":"+config.SMTPPort, auth, config.SMTPFrom, to, []byte(message))
}

// ErrSMTPNotConfigured is returned by SendTestMail when an SMTP setting is missing.
var ErrSMTPNotConfigured = errors.New("SMTP is not configured: set SMTP_HOST, SMTP_USERNAME, SMTP_PASSWORD and SMTP_FROM")

func smtpConfigured() bool {
	return config.SMTPHost != "" && config.SMTPUsername != "" && config.SMTPPassword != "" && config.SMTPFrom != ""
}

type alertsOffKey struct{}

// WithoutAlerts marks ctx so Notify and Mail do nothing: a backfill writes data without telling
// anyone about it.
func WithoutAlerts(ctx context.Context) context.Context {
	return context.WithValue(ctx, alertsOffKey{}, true)
}

func alertsOff(ctx context.Context) bool {
	off, _ := ctx.Value(alertsOffKey{}).(bool)
	return off
}

// RecordEvent adds a step to the lead's timeline. A failure is logged, not returned: the action
// it describes has already happened.
func RecordEvent(ctx context.Context, program, leadID, eventType string, actor models.Actor, at int64, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	event := models.Event{
		ID: core.NewID(), Program: program, LeadID: leadID, Type: eventType, Actor: actor, At: at, Data: data,
		Created: models.Created{At: nowSeconds(), By: actor.Email},
	}
	if err := store.InsertEvent(ctx, event); err != nil {
		log.Printf("salesAudit: recording %s event on lead %s: %v", eventType, leadID, err)
	}
}

// Notify adds an in-app notification for each recipient; failures are logged.
func Notify(ctx context.Context, program string, recipients []string, template models.Notification) {
	if alertsOff(ctx) {
		return
	}
	now := nowSeconds()
	for _, email := range core.NonEmpty(recipients...) {
		notification := template
		notification.ID = core.NewID()
		notification.Program = program
		notification.RecipientEmail = strings.ToLower(email)
		notification.Created = models.Created{At: now, By: template.Created.By}
		if err := store.InsertNotification(ctx, notification); err != nil {
			log.Printf("salesAudit: notifying %s: %v", email, err)
		}
	}
}

// Mail logs a mail and queues it. The mail stays logged even if queueing fails, so the UI still
// shows that it was raised.
func Mail(ctx context.Context, program, by, leadID, kind, trigger string, to []string, subject, body string) (models.Mail, error) {
	if alertsOff(ctx) {
		return models.Mail{}, nil
	}
	now := nowSeconds()
	mail := models.Mail{To: core.NonEmpty(to...), Subject: subject, Trigger: trigger, SentAt: now}
	alert := models.Alert{
		ID: core.NewID(), Program: program, LeadID: leadID, Kind: kind, Mail: mail, Body: body,
		Delivery: models.DeliveryQueued, Created: models.Created{At: now, By: by},
	}
	if err := store.InsertAlert(ctx, alert); err != nil {
		return mail, err
	}
	if err := EnqueueMail(program, alert.ID); err != nil {
		log.Printf("salesAudit: queueing mail %s failed: %v", alert.ID, err)
		if err := store.SetAlertDelivery(ctx, program, alert.ID, models.DeliveryFailed); err != nil {
			log.Printf("salesAudit: marking mail %s failed: %v", alert.ID, err)
		}
	}
	return mail, nil
}

// DeliverMail sends one logged mail. Without SMTP settings it stays logged and is marked skipped.
func DeliverMail(ctx context.Context, program, alertID string) error {
	alert, err := store.FindAlert(ctx, program, alertID)
	if store.IsNotFound(err) {
		log.Printf("salesAudit: mail %s not found, dropping", alertID)
		return nil
	}
	if err != nil {
		return err
	}
	if !smtpConfigured() || len(alert.To) == 0 {
		log.Printf("salesAudit: not sending %q to %v (SMTP not configured or no recipients)", alert.Subject, alert.To)
		return store.SetAlertDelivery(ctx, program, alertID, models.DeliverySkipped)
	}
	if err := SendSMTP(alert.To, alert.Subject, alert.Body); err != nil {
		if markErr := store.SetAlertDelivery(ctx, program, alertID, models.DeliveryFailed); markErr != nil {
			log.Printf("salesAudit: marking mail %s failed: %v", alertID, markErr)
		}
		return fmt.Errorf("sending mail %s: %w", alertID, err)
	}
	return store.SetAlertDelivery(ctx, program, alertID, models.DeliverySent)
}

// SendTestMail sends one mail straight over SMTP, skipping the queue and the log.
func SendTestMail(to string) error {
	if !smtpConfigured() {
		return ErrSMTPNotConfigured
	}
	return SendSMTP([]string{to}, "Zen Sales Audit test mail", core.TestMailBody(Now()))
}

// Notifications of the signed-in member.

type NotificationList struct {
	Items  []models.Notification `json:"items"`
	Unread int                   `json:"unread"`
}

func ListNotifications(ctx context.Context, member models.Member, unreadOnly bool) (NotificationList, error) {
	items, unread, err := store.FindNotifications(ctx, member.Program, member.Email, unreadOnly, 100)
	return NotificationList{Items: items, Unread: unread}, err
}

func ReadNotification(ctx context.Context, member models.Member, notificationID string) error {
	return orNotFound(store.MarkNotificationRead(ctx, member.Program, member.Email, notificationID, nowSeconds()), "Notification")
}

func ReadAllNotifications(ctx context.Context, member models.Member) error {
	return store.MarkAllNotificationsRead(ctx, member.Program, member.Email, nowSeconds())
}
