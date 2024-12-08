package database

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

type Roundtrip struct {
	ID           uuid.UUID `gorm:"primarykey"`
	ConnectionId uuid.UUID `gorm:"index"`
	ActorId      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}
