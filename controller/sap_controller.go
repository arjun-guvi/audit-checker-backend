package controller

import (
	"auditApp/config"
	"auditApp/models"
	"auditApp/worker"
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SAPList returns all SAP records
func SAPList(c *gin.Context) {
	collection := config.MongoDB.Collection(models.SAPCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Filter options
	filter := bson.M{}
	if stage := c.Query("stage"); stage != "" {
		filter["Stage"] = stage
	}
	if email := c.Query("email"); email != "" {
		filter["Email"] = bson.M{"$regex": email, "$options": "i"}
	}
	if isActive := c.Query("is_active"); isActive != "" {
		filter["is_active"] = isActive == "true"
	}

	// Pagination
	pageSize := int64(20)
	if size := c.Query("size"); size != "" {
		if parsed, err := time.ParseDuration(size); err == nil {
			_ = parsed // just to avoid unused variable error
		}
	}

	opts := options.Find().SetSort(bson.D{{Key: "modified_time", Value: -1}}).SetLimit(pageSize)

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch SAP records",
		})
		return
	}
	defer cursor.Close(ctx)

	var sapRecords []models.SAP
	if err = cursor.All(ctx, &sapRecords); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to parse SAP records",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   sapRecords,
		"count":  len(sapRecords),
	})
}

// SAPGetByID returns a single SAP record
func SAPGetByID(c *gin.Context) {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid ID format",
		})
		return
	}

	collection := config.MongoDB.Collection(models.SAPCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var sap models.SAP
	err = collection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&sap)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "SAP record not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   sap,
	})
}

// SAPEdit updates an existing SAP record
func SAPEdit(c *gin.Context) {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid ID format",
		})
		return
	}

	var sap models.SAP
	if err := c.ShouldBindJSON(&sap); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body",
		})
		return
	}

	collection := config.MongoDB.Collection(models.SAPCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"updated_at": time.Now(),
		},
	}

	// Build update fields dynamically
	updateData := update["$set"].(bson.M)
	if sap.Stage != "" {
		updateData["Stage"] = sap.Stage
	}
	if sap.Status != "" {
		updateData["status"] = sap.Status
	}
	if sap.Email != "" {
		updateData["Email"] = sap.Email
	}
	if sap.StudentFullName != "" {
		updateData["Student_Full_Name"] = sap.StudentFullName
	}
	if sap.PrimaryPhone != "" {
		updateData["Primary_Phone"] = sap.PrimaryPhone
	}
	if sap.PaymentStatus != "" {
		updateData["Payment_Status"] = sap.PaymentStatus
	}
	if sap.TotalPaid != "" {
		updateData["Total_Paid"] = sap.TotalPaid
	}
	if sap.BalanceAmount != "" {
		updateData["Balance_Amount"] = sap.BalanceAmount
	}
	if len(sap.MailsToSendTo) > 0 {
		updateData["mails_to_send_to"] = sap.MailsToSendTo
	}
	if sap.ReminderIntervalMinutes > 0 {
		updateData["reminder_interval_minutes"] = sap.ReminderIntervalMinutes
	}

	opts := options.Update().SetUpsert(false)
	result, err := collection.UpdateOne(ctx, bson.M{"_id": objectID}, update, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to update SAP record",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "SAP record not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "SAP record updated successfully",
	})
}

// SAPDelete deletes an SAP record
func SAPDelete(c *gin.Context) {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid ID format",
		})
		return
	}

	collection := config.MongoDB.Collection(models.SAPCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := collection.DeleteOne(ctx, bson.M{"_id": objectID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete SAP record",
		})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "SAP record not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "SAP record deleted successfully",
	})
}

// SAPCreate creates a new SAP record
func SAPCreate(c *gin.Context) {
	var sap models.SAP
	if err := c.ShouldBindJSON(&sap); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body",
		})
		return
	}

	now := time.Now()
	sap.CreatedAt = now
	sap.UpdatedAt = now
	
	// Set defaults
	if sap.ReminderIntervalMinutes == 0 {
		sap.ReminderIntervalMinutes = 30 // Default 30 minutes
	}
	if len(sap.MailsToSendTo) == 0 && sap.Email != "" {
		sap.MailsToSendTo = []string{sap.Email}
	}
	sap.IsActive = true

	collection := config.MongoDB.Collection(models.SAPCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := collection.InsertOne(ctx, sap)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create SAP record",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": "SAP record created successfully",
		"id":      result.InsertedID,
	})
}

// SAPAddRecipients adds email recipients to a SAP record
func SAPAddRecipients(c *gin.Context) {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid ID format",
		})
		return
	}

	var req struct {
		Emails []string `json:"emails" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body, emails array required",
		})
		return
	}

	collection := config.MongoDB.Collection(models.SAPCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	update := bson.M{
		"$addToSet": bson.M{
			"mails_to_send_to": bson.M{"$each": req.Emails},
		},
		"$set": bson.M{
			"updated_at": time.Now(),
		},
	}

	result, err := collection.UpdateOne(ctx, bson.M{"_id": objectID}, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to add recipients",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "SAP record not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Recipients added successfully",
		"added":   len(req.Emails),
	})
}

// SAPTriggerReminder manually triggers a reminder for a SAP record
func SAPTriggerReminder(c *gin.Context) {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid ID format",
		})
		return
	}

	collection := config.MongoDB.Collection(models.SAPCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var sap models.SAP
	err = collection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&sap)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "SAP record not found",
		})
		return
	}

	if len(sap.MailsToSendTo) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "No recipients configured for this SAP record",
		})
		return
	}

	// Import and use the worker's email sending
	worker := worker.NewSAPWorker()
	if err := worker.SendReminderEmail(sap, sap.MailsToSendTo); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to send reminder: " + err.Error(),
		})
		return
	}

	// Update reminder status
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"last_reminder_sent": now,
			"updated_at":         now,
		},
		"$inc": bson.M{
			"reminder_count": 1,
		},
	}
	collection.UpdateOne(ctx, bson.M{"_id": objectID}, update)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Reminder sent successfully",
	})
}

// SAPActivate activates a SAP record for reminders
func SAPActivate(c *gin.Context) {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid ID format",
		})
		return
	}

	collection := config.MongoDB.Collection(models.SAPCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"is_active":          true,
			"next_reminder_due": now,
			"updated_at":        now,
		},
	}

	result, err := collection.UpdateOne(ctx, bson.M{"_id": objectID}, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to activate SAP record",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "SAP record not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "SAP record activated for reminders",
	})
}

// SAPDeactivate deactivates a SAP record from reminders
func SAPDeactivate(c *gin.Context) {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid ID format",
		})
		return
	}

	collection := config.MongoDB.Collection(models.SAPCollection())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"is_active":   false,
			"updated_at": time.Now(),
		},
	}

	result, err := collection.UpdateOne(ctx, bson.M{"_id": objectID}, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to deactivate SAP record",
		})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "SAP record not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "SAP record deactivated from reminders",
	})
}
