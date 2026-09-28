package service

import (
	"context"
	"fmt"
	"time"

	"github.com/wyw14/solo-038/internal/dryer"
	"github.com/wyw14/solo-038/internal/model"
	"github.com/wyw14/solo-038/internal/trial"
)

// defaultProductionParameters matches the settings the press has always run
// with; a trial study has to be approved and applied before they change.
func defaultProductionParameters() trial.Parameters {
	return trial.Parameters{Speed: 8, Profile: dryer.Profile{Name: "production", Zones: []dryer.Zone{{Name: "entry", Temperature: 80, Airflow: 20}, {Name: "exit", Temperature: 100, Airflow: 24}}}}
}

// ProductionParameters is the live production configuration. Trial groups
// never touch it; only applying an approved study does.
func (runtime *Runtime) ProductionParameters() trial.Parameters {
	return runtime.trials.Production()
}

func (runtime *Runtime) TrialStudies() []trial.Study {
	return runtime.trials.Studies()
}

func (runtime *Runtime) TrialStudy(id string) (trial.Study, bool) {
	return runtime.trials.Study(id)
}

func (runtime *Runtime) CreateTrialStudy(ctx context.Context, criteria trial.Criteria) (trial.Study, error) {
	select {
	case <-ctx.Done():
		return trial.Study{}, ctx.Err()
	default:
	}
	current, ok := runtime.jobs.Current()
	if !ok {
		return trial.Study{}, fmt.Errorf("there is no active job")
	}
	return runtime.trials.CreateStudy(current, criteria, time.Now())
}

func (runtime *Runtime) AddTrialGroup(studyID, label string, parameters trial.Parameters) (trial.Group, error) {
	return runtime.trials.AddGroup(studyID, label, parameters, time.Now())
}

func (runtime *Runtime) RecordTrialResult(studyID, groupID string, result trial.Result) (trial.Study, error) {
	result.RecordedAt = time.Now().UTC()
	return runtime.trials.RecordResult(studyID, groupID, result, time.Now())
}

func (runtime *Runtime) EvaluateTrialStudy(studyID string) (trial.Comparison, error) {
	study, ok := runtime.trials.Study(studyID)
	if !ok {
		return trial.Comparison{}, fmt.Errorf("trial study %q was not found", studyID)
	}
	return trial.Compare(study, time.Now()), nil
}

func (runtime *Runtime) ApproveTrialGroup(studyID, groupID string) (trial.Study, error) {
	return runtime.trials.Approve(studyID, groupID, time.Now())
}

// ApplyTrialStudy puts the approved group into production. It refuses to run
// when the job has moved on to another edition since the study was approved.
func (runtime *Runtime) ApplyTrialStudy(ctx context.Context, studyID string) (trial.Study, error) {
	select {
	case <-ctx.Done():
		return trial.Study{}, ctx.Err()
	default:
	}
	study, ok := runtime.trials.Study(studyID)
	if !ok {
		return trial.Study{}, fmt.Errorf("trial study %q was not found", studyID)
	}
	current, ok := runtime.jobs.Current()
	if !ok {
		return trial.Study{}, fmt.Errorf("there is no active job")
	}
	if current.ID != study.JobID || current.PlateGeneration != study.PlateGeneration {
		return trial.Study{}, fmt.Errorf("job edition moved past plate generation %d, trial study cannot be applied", study.PlateGeneration)
	}
	applied, adopted, err := runtime.trials.Apply(studyID, time.Now())
	if err != nil {
		return trial.Study{}, err
	}
	payload := map[string]any{"study_id": applied.ID, "group_id": adopted.ID, "speed": adopted.Parameters.Speed, "profile": adopted.Parameters.Profile.Name}
	event, err := model.NewEvent(model.NewOperationID(), current, "trial_applied", payload, time.Now())
	if err != nil {
		runtime.RecordIncident("warning", "trial", fmt.Sprintf("trial applied event could not be built: %v", err))
		return applied, nil
	}
	if _, err := runtime.journal.Append(event); err != nil {
		runtime.RecordIncident("warning", "trial", fmt.Sprintf("trial applied event was not journaled: %v", err))
	}
	return applied, nil
}
