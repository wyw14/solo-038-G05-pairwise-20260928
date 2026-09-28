package service

import (
	"fmt"
	"time"

	"github.com/wyw14/solo-038/internal/dryer"
	"github.com/wyw14/solo-038/internal/model"
	"github.com/wyw14/solo-038/internal/trial"
)

type CreateTrialInput struct {
	Name     string
	Criteria trial.Criteria
}

type AddGroupInput struct {
	Kind     string
	Name     string
	Settings trial.Settings
}

type RecordResultInput struct {
	Speed               float64
	Profile             dryer.Profile
	RegisterDeviationMM []float64
	WasteSheets         int
}

// CreateTrial opens a comparison campaign on the current plate generation.
// The baseline group snapshots the formal production settings; creating or
// running trial groups never changes those settings.
func (runtime *Runtime) CreateTrial(input CreateTrialInput) (trial.Trial, error) {
	current, ok := runtime.jobs.Current()
	if !ok {
		return trial.Trial{}, fmt.Errorf("there is no active job")
	}
	plateGeneration := runtime.currentPlateGeneration(current)
	if plateGeneration == 0 {
		return trial.Trial{}, fmt.Errorf("there is no active plate generation to trial")
	}
	baseline := runtime.productionSettings(current, plateGeneration)
	now := time.Now()
	created, err := runtime.trials.Open(current, plateGeneration, input.Name, input.Criteria, baseline, now)
	if err != nil {
		return trial.Trial{}, err
	}
	if err := trial.PersistOpened(runtime.journal, current, model.NewOperationID(), created, now); err != nil {
		return trial.Trial{}, err
	}
	return created, nil
}

func (runtime *Runtime) AddTrialGroup(trialID model.TrialID, input AddGroupInput) (trial.Trial, trial.Group, error) {
	current, ok := runtime.jobs.Current()
	if !ok {
		return trial.Trial{}, trial.Group{}, fmt.Errorf("there is no active job")
	}
	now := time.Now()
	updated, group, err := runtime.trials.AddGroup(trialID, trial.GroupKind(input.Kind), input.Name, input.Settings, now)
	if err != nil {
		return trial.Trial{}, trial.Group{}, err
	}
	if err := trial.PersistGroupAdded(runtime.journal, current, model.NewOperationID(), updated, group, now); err != nil {
		return trial.Trial{}, trial.Group{}, err
	}
	return updated, group, nil
}

// RecordTrialResult stores one trial run (speed, dryer curve, register
// deviation, waste sheets) against a group. Production settings are untouched.
func (runtime *Runtime) RecordTrialResult(trialID model.TrialID, groupID model.GroupID, input RecordResultInput) (trial.Trial, error) {
	current, ok := runtime.jobs.Current()
	if !ok {
		return trial.Trial{}, fmt.Errorf("there is no active job")
	}
	register, err := trial.NewRegisterResult(input.RegisterDeviationMM)
	if err != nil {
		return trial.Trial{}, err
	}
	result := trial.Result{Speed: input.Speed, Profile: input.Profile, Register: register, WasteSheets: input.WasteSheets}
	now := time.Now()
	updated, err := runtime.trials.RecordResult(trialID, groupID, result, now)
	if err != nil {
		return trial.Trial{}, err
	}
	group, _, found := updatedGroup(updated, groupID)
	if !found {
		return trial.Trial{}, trial.ErrGroupNotFound
	}
	if err := trial.PersistResultRecorded(runtime.journal, current, model.NewOperationID(), updated, group, result, now); err != nil {
		return trial.Trial{}, err
	}
	return updated, nil
}

func (runtime *Runtime) TrialComparison(trialID model.TrialID) (trial.Report, error) {
	found, ok := runtime.trials.Get(trialID)
	if !ok {
		return trial.Report{}, trial.ErrTrialNotFound
	}
	return trial.Compare(found)
}

// ApproveTrial puts the selected group's settings into formal production use.
// Groups that were not adopted keep their recorded data.
func (runtime *Runtime) ApproveTrial(trialID model.TrialID, groupID model.GroupID, approvedBy string) (trial.Trial, trial.ProductionSettings, error) {
	current, ok := runtime.jobs.Current()
	if !ok {
		return trial.Trial{}, trial.ProductionSettings{}, fmt.Errorf("there is no active job")
	}
	now := time.Now()
	updated, err := runtime.trials.Approve(trialID, groupID, approvedBy, now)
	if err != nil {
		return trial.Trial{}, trial.ProductionSettings{}, err
	}
	group, _, found := updatedGroup(updated, groupID)
	if !found {
		return trial.Trial{}, trial.ProductionSettings{}, trial.ErrGroupNotFound
	}
	settings := runtime.production.Adopt(updated, group, now)
	if err := trial.PersistApproved(runtime.journal, current, model.NewOperationID(), updated, settings, now); err != nil {
		return trial.Trial{}, trial.ProductionSettings{}, err
	}
	return updated, settings, nil
}

func (runtime *Runtime) ListTrials() []trial.Trial {
	return runtime.trials.List()
}

func (runtime *Runtime) GetTrial(trialID model.TrialID) (trial.Trial, bool) {
	return runtime.trials.Get(trialID)
}

// ProductionConfig exposes the formal production settings for the current
// plate generation.
func (runtime *Runtime) ProductionConfig() (trial.ProductionSettings, bool) {
	current, ok := runtime.jobs.Current()
	if !ok {
		return trial.ProductionSettings{}, false
	}
	return runtime.production.Get(current.ID, runtime.currentPlateGeneration(current))
}

// productionSettings resolves the formal settings for a plate generation,
// falling back to the current equipment defaults before any trial approval.
func (runtime *Runtime) productionSettings(current model.Job, plateGeneration uint64) trial.Settings {
	if settings, ok := runtime.production.Get(current.ID, plateGeneration); ok {
		return settings.Settings
	}
	speed := 8.0
	_, _, _, feedSpeed := runtime.unwind.Snapshot()
	if feedSpeed > 0 {
		speed = feedSpeed
	}
	return trial.Settings{Speed: speed, Profile: trial.DefaultProfile()}
}

func (runtime *Runtime) currentPlateGeneration(current model.Job) uint64 {
	_, plateGeneration, _, _ := runtime.press.Snapshot()
	if plateGeneration == 0 {
		plateGeneration = current.PlateGeneration
	}
	return plateGeneration
}

func updatedGroup(value trial.Trial, groupID model.GroupID) (trial.Group, int, bool) {
	for index, group := range value.Groups {
		if group.ID == groupID {
			return group, index, true
		}
	}
	return trial.Group{}, -1, false
}
