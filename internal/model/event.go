package model

import (
	"time"
)

type EventState string

const (
	EventStatePrepare    EventState = "preparing"
	EventStateInProgress            = "inprogress"
	EventStateCompleted             = "completed"
	EventStateCancelled             = "cancelled"
)

type Event struct {
	// unique ID for this event
	ID uint `json:"id" gorm:"primaryKey"`

	CoverImg string `json:"cover_img"`

	Metadata string `json:"metadata"`

	// Member init this application
	Initiator string `json:"applicant"`

	// Information of the event
	Title   string    `json:"title"`
	Content string    `json:"content"`
	StartAt time.Time `json:"start_at"`
	EndAt   time.Time `json:"end_at"`

	RewardDetail string `json:"reward_detail"`

	// Application state, which contains open/approved/rejected/processing/completed
	State EventState `json:"state"`

	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}
