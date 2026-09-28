package trial

import (
	"time"

	"github.com/wyw14/solo-038/internal/journal"
	"github.com/wyw14/solo-038/internal/model"
)

// Trial events are journaled so decided trials — including the groups that
// were not adopted — survive restarts.
func persistEvent(store *journal.Store, job model.Job, operation model.OperationID, kind string, payload any, now time.Time) error {
	event, err := model.NewEvent(operation, job, kind, payload, now)
	if err != nil {
		return err
	}
	_, err = store.Append(event)
	return err
}

func PersistOpened(store *journal.Store, job model.Job, operation model.OperationID, trial Trial, now time.Time) error {
	return persistEvent(store, job, operation, "trial_opened", trial, now)
}

func PersistGroupAdded(store *journal.Store, job model.Job, operation model.OperationID, trial Trial, group Group, now time.Time) error {
	return persistEvent(store, job, operation, "trial_group_added", map[string]any{"trial_id": trial.ID, "group": group}, now)
}

func PersistResultRecorded(store *journal.Store, job model.Job, operation model.OperationID, trial Trial, group Group, result Result, now time.Time) error {
	return persistEvent(store, job, operation, "trial_result_recorded", map[string]any{"trial_id": trial.ID, "group_id": group.ID, "result": result}, now)
}

func PersistApproved(store *journal.Store, job model.Job, operation model.OperationID, trial Trial, settings ProductionSettings, now time.Time) error {
	return persistEvent(store, job, operation, "trial_approved", map[string]any{"trial": trial, "production_settings": settings}, now)
}
