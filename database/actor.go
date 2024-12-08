package database

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

type Actor struct {
	ID         uuid.UUID `gorm:"primarykey"`
	ExternalId string    `gorm:"unique,index"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}
