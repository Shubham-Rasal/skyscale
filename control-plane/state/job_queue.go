package state

import "time"

// JobQueueItem represents a queued GPU training job.
type JobQueueItem struct {
	ID              string `gorm:"primaryKey"`
	UserID          string `gorm:"index"`
	ExecutionID     string
	DockerImage     string
	GPUModel        string
	ProviderName    string
	EnvVarsJSON     string // JSON-encoded map[string]string
	ControlPlaneURL string
	Priority        int
	Status          string `gorm:"index"` // queued | dispatching | done | failed
	Error           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
