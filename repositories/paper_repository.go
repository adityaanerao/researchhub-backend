package repositories

import (
	"researchhub-backend/config"
	"researchhub-backend/models"

	"gorm.io/gorm"
)

func PreloadPaperFull(db *gorm.DB) *gorm.DB {
	return db.
		Preload("Category").
		Preload("Student").
		Preload("Authors", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("author_order ASC")
		}).
		Preload("Files", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("version_number DESC, created_at DESC")
		}).
		Preload("StatusHistory", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("created_at ASC")
		}).
		Preload("Assignments.Reviewer").
		Preload("Assignments.Review").
		Preload("Reviews.Reviewer").
		Preload("Revisions", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("revision_round DESC")
		}).
		Preload("Decisions", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("decided_at DESC")
		}).
		Preload("Publication")
}

func RecordPaperStatusChange(tx *gorm.DB, paperID, fromStatus, toStatus, userID, userName, userRole, remarks string) error {
	if tx == nil {
		tx = config.DB
	}
	history := models.PaperStatusHistory{
		PaperID:         paperID,
		FromStatus:      fromStatus,
		ToStatus:        toStatus,
		ChangedByUserID: userID,
		ChangedByName:   userName,
		ChangedByRole:   userRole,
		Remarks:         remarks,
	}
	return tx.Create(&history).Error
}
