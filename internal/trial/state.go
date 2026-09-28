package trial

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/wyw14/solo-038/internal/dryer"
	"github.com/wyw14/solo-038/internal/model"
)

var (
	ErrTrialNotFound      = errors.New("trial was not found")
	ErrGroupNotFound      = errors.New("trial group was not found")
	ErrTrialOpen          = errors.New("an open trial already exists for this plate generation")
	ErrTrialDecided       = errors.New("trial is already approved and closed")
	ErrBaselineExists     = errors.New("trial already has a baseline group")
	ErrGroupOpen          = errors.New("another group is still recording results")
	ErrNoResults          = errors.New("trial group has no recorded results")
	ErrNotEnoughGroups    = errors.New("comparison needs results from at least two groups")
	ErrGroupNotRecordable = errors.New("trial group is not open for recording")
)

type GroupKind string

const (
	GroupBaseline   GroupKind = "baseline"
	GroupExperiment GroupKind = "experiment"
)

type GroupStatus string

const (
	GroupOpen       GroupStatus = "open"
	GroupRecorded   GroupStatus = "recorded"
	GroupApproved   GroupStatus = "approved"
	GroupNotAdopted GroupStatus = "not_adopted"
)

type TrialStatus string

const (
	TrialOpen     TrialStatus = "open"
	TrialApproved TrialStatus = "approved"
)

// Settings is one tunable parameter set: press speed and the dryer curve.
type Settings struct {
	Speed   float64       `json:"speed"`
	Profile dryer.Profile `json:"profile"`
}

func (settings Settings) Validate() error {
	if settings.Speed <= 0 {
		return fmt.Errorf("trial settings speed must be positive")
	}
	if err := settings.Profile.Validate(); err != nil {
		return err
	}
	return nil
}

// RegisterResult summarises registration stability for one trial run.
type RegisterResult struct {
	MaxDeviation    float64 `json:"max_deviation"`
	MeanDeviation   float64 `json:"mean_deviation"`
	StddevDeviation float64 `json:"stddev_deviation"`
}

// Result is one recorded trial run against a group.
type Result struct {
	Speed       float64        `json:"speed"`
	Profile     dryer.Profile  `json:"profile"`
	Register    RegisterResult `json:"register"`
	WasteSheets int            `json:"waste_sheets"`
	RecordedAt  time.Time      `json:"recorded_at"`
}

type Group struct {
	ID        model.GroupID `json:"id"`
	Kind      GroupKind     `json:"kind"`
	Name      string        `json:"name"`
	Settings  Settings      `json:"settings"`
	Status    GroupStatus   `json:"status"`
	Results   []Result      `json:"results"`
	CreatedAt time.Time     `json:"created_at"`
}

func (group Group) validate() error {
	if !group.ID.Valid() {
		return fmt.Errorf("group id is invalid")
	}
	if group.Kind != GroupBaseline && group.Kind != GroupExperiment {
		return fmt.Errorf("group kind is invalid")
	}
	if group.Name == "" {
		return fmt.Errorf("group name is required")
	}
	return group.Settings.Validate()
}

// Trial is one comparison campaign bound to a job and a plate generation (版次).
type Trial struct {
	ID              model.TrialID `json:"id"`
	JobID           model.JobID   `json:"job_id"`
	JobGeneration   uint64        `json:"job_generation"`
	PlateGeneration uint64        `json:"plate_generation"`
	Name            string        `json:"name"`
	Criteria        Criteria      `json:"criteria"`
	Groups          []Group       `json:"groups"`
	Status          TrialStatus   `json:"status"`
	ApprovedGroup   model.GroupID `json:"approved_group,omitempty"`
	ApprovedBy      string        `json:"approved_by,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	DecidedAt       time.Time     `json:"decided_at,omitempty"`
}

func (trial Trial) validate() error {
	if !trial.ID.Valid() || !trial.JobID.Valid() {
		return fmt.Errorf("trial identity is invalid")
	}
	if trial.JobGeneration == 0 || trial.PlateGeneration == 0 {
		return fmt.Errorf("trial generations are required")
	}
	if trial.Name == "" {
		return fmt.Errorf("trial name is required")
	}
	return trial.Criteria.Validate()
}

func (trial Trial) group(groupID model.GroupID) (Group, int, bool) {
	for index, group := range trial.Groups {
		if group.ID == groupID {
			return group, index, true
		}
	}
	return Group{}, -1, false
}

func (trial Trial) hasOpenGroup() bool {
	for _, group := range trial.Groups {
		if group.Status == GroupOpen {
			return true
		}
	}
	return false
}

// State holds every trial campaign; decided trials and their unadopted
// groups are retained here and in the journal.
type State struct {
	mu     sync.RWMutex
	trials map[model.TrialID]Trial
}

func NewState() *State {
	return &State{trials: map[model.TrialID]Trial{}}
}

func (state *State) Open(job model.Job, plateGeneration uint64, name string, criteria Criteria, baseline Settings, now time.Time) (Trial, error) {
	criteria = criteria.withDefaults()
	if err := baseline.Validate(); err != nil {
		return Trial{}, err
	}
	trial := Trial{
		ID:              model.NewTrialID(),
		JobID:           job.ID,
		JobGeneration:   job.Generation,
		PlateGeneration: plateGeneration,
		Name:            name,
		Criteria:        criteria,
		Status:          TrialOpen,
		CreatedAt:       now.UTC(),
	}
	if trial.Name == "" {
		trial.Name = fmt.Sprintf("plate-%d-trial", plateGeneration)
	}
	if err := trial.validate(); err != nil {
		return Trial{}, err
	}
	baselineGroup := Group{
		ID:        model.NewGroupID(),
		Kind:      GroupBaseline,
		Name:      "baseline",
		Settings:  baseline,
		Status:    GroupOpen,
		CreatedAt: now.UTC(),
	}
	if err := baselineGroup.validate(); err != nil {
		return Trial{}, err
	}
	trial.Groups = []Group{baselineGroup}
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, existing := range state.trials {
		if existing.JobID == trial.JobID && existing.PlateGeneration == plateGeneration && existing.Status == TrialOpen {
			return Trial{}, ErrTrialOpen
		}
	}
	state.trials[trial.ID] = trial
	return trial, nil
}

func (state *State) AddGroup(trialID model.TrialID, kind GroupKind, name string, settings Settings, now time.Time) (Trial, Group, error) {
	if kind == "" {
		kind = GroupExperiment
	}
	if err := settings.Validate(); err != nil {
		return Trial{}, Group{}, err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	trial, ok := state.trials[trialID]
	if !ok {
		return Trial{}, Group{}, ErrTrialNotFound
	}
	if trial.Status != TrialOpen {
		return Trial{}, Group{}, ErrTrialDecided
	}
	if kind == GroupBaseline {
		return Trial{}, Group{}, ErrBaselineExists
	}
	if trial.hasOpenGroup() {
		return Trial{}, Group{}, ErrGroupOpen
	}
	if name == "" {
		name = fmt.Sprintf("experiment-%d", len(trial.Groups))
	}
	group := Group{ID: model.NewGroupID(), Kind: kind, Name: name, Settings: settings, Status: GroupOpen, CreatedAt: now.UTC()}
	if err := group.validate(); err != nil {
		return Trial{}, Group{}, err
	}
	trial.Groups = append(trial.Groups, group)
	state.trials[trial.ID] = trial
	return trial, group, nil
}

func (state *State) RecordResult(trialID model.TrialID, groupID model.GroupID, result Result, now time.Time) (Trial, error) {
	if err := result.validate(); err != nil {
		return Trial{}, err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	trial, ok := state.trials[trialID]
	if !ok {
		return Trial{}, ErrTrialNotFound
	}
	if trial.Status != TrialOpen {
		return Trial{}, ErrTrialDecided
	}
	group, index, found := trial.group(groupID)
	if !found {
		return Trial{}, ErrGroupNotFound
	}
	if group.Status != GroupOpen {
		return Trial{}, ErrGroupNotRecordable
	}
	result.RecordedAt = now.UTC()
	group.Results = append(group.Results, result)
	group.Status = GroupRecorded
	trial.Groups[index] = group
	state.trials[trial.ID] = trial
	return trial, nil
}

// Approve marks the selected group as adopted and every other group as
// not adopted; all groups and their results stay recorded.
func (state *State) Approve(trialID model.TrialID, groupID model.GroupID, approvedBy string, now time.Time) (Trial, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	trial, ok := state.trials[trialID]
	if !ok {
		return Trial{}, ErrTrialNotFound
	}
	if trial.Status != TrialOpen {
		return Trial{}, ErrTrialDecided
	}
	_, index, found := trial.group(groupID)
	if !found {
		return Trial{}, ErrGroupNotFound
	}
	if len(trial.Groups[index].Results) == 0 {
		return Trial{}, ErrNoResults
	}
	for other := range trial.Groups {
		if trial.Groups[other].ID == groupID {
			trial.Groups[other].Status = GroupApproved
		} else {
			trial.Groups[other].Status = GroupNotAdopted
		}
	}
	trial.Status = TrialApproved
	trial.ApprovedGroup = groupID
	trial.ApprovedBy = approvedBy
	trial.DecidedAt = now.UTC()
	state.trials[trial.ID] = trial
	return trial, nil
}

func (state *State) Get(trialID model.TrialID) (Trial, bool) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	trial, ok := state.trials[trialID]
	return trial, ok
}

func (state *State) List() []Trial {
	state.mu.RLock()
	defer state.mu.RUnlock()
	trials := make([]Trial, 0, len(state.trials))
	for _, trial := range state.trials {
		trials = append(trials, trial)
	}
	sort.Slice(trials, func(left, right int) bool {
		if trials[left].CreatedAt.Equal(trials[right].CreatedAt) {
			return trials[left].ID < trials[right].ID
		}
		return trials[left].CreatedAt.Before(trials[right].CreatedAt)
	})
	return trials
}

func (state *State) OpenForPlate(jobID model.JobID, plateGeneration uint64) (Trial, bool) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	for _, trial := range state.trials {
		if trial.JobID == jobID && trial.PlateGeneration == plateGeneration && trial.Status == TrialOpen {
			return trial, true
		}
	}
	return Trial{}, false
}
