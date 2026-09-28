package trial

import (
	"sync"
	"time"

	"github.com/wyw14/solo-038/internal/model"
)

// ProductionSettings are the formal production parameters for one plate
// generation. Trial groups never write here; only an approved group does.
type ProductionSettings struct {
	JobID           model.JobID   `json:"job_id"`
	PlateGeneration uint64        `json:"plate_generation"`
	Settings        Settings      `json:"settings"`
	Source          string        `json:"source"`
	TrialID         model.TrialID `json:"trial_id,omitempty"`
	GroupID         model.GroupID `json:"group_id,omitempty"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

type productionKey struct {
	jobID model.JobID
	plate uint64
}

// ProductionStore holds the formal per-plate production configuration.
type ProductionStore struct {
	mu       sync.RWMutex
	settings map[productionKey]ProductionSettings
}

func NewProductionStore() *ProductionStore {
	return &ProductionStore{settings: map[productionKey]ProductionSettings{}}
}

func (store *ProductionStore) Get(jobID model.JobID, plateGeneration uint64) (ProductionSettings, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	settings, ok := store.settings[productionKey{jobID: jobID, plate: plateGeneration}]
	return settings, ok
}

// Adopt puts an approved group's settings into formal production use.
func (store *ProductionStore) Adopt(trial Trial, group Group, now time.Time) ProductionSettings {
	settings := ProductionSettings{
		JobID:           trial.JobID,
		PlateGeneration: trial.PlateGeneration,
		Settings:        group.Settings,
		Source:          "trial",
		TrialID:         trial.ID,
		GroupID:         group.ID,
		UpdatedAt:       now.UTC(),
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.settings[productionKey{jobID: trial.JobID, plate: trial.PlateGeneration}] = settings
	return settings
}
