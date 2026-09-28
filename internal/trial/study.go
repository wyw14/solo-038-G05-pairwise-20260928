package trial

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/wyw14/solo-038/internal/dryer"
	"github.com/wyw14/solo-038/internal/model"
)

type GroupKind string

const (
	GroupBaseline   GroupKind = "baseline"
	GroupExperiment GroupKind = "experiment"
)

type StudyStatus string

const (
	StudyOpen     StudyStatus = "open"
	StudyApproved StudyStatus = "approved"
	StudyApplied  StudyStatus = "applied"
)

type GroupStatus string

const (
	GroupCollecting GroupStatus = "collecting"
	GroupComplete   GroupStatus = "complete"
	GroupAdopted    GroupStatus = "adopted"
	GroupNotAdopted GroupStatus = "not_adopted"
)

// Parameters are the machine settings one trial group runs with: web speed
// and the dryer curve. They mirror the shape of the production configuration
// so an approved group can be applied without translation.
type Parameters struct {
	Speed   float64       `json:"speed"`
	Profile dryer.Profile `json:"profile"`
}

func (parameters Parameters) Validate() error {
	if parameters.Speed <= 0 || parameters.Speed > 100 || math.IsNaN(parameters.Speed) {
		return fmt.Errorf("trial speed is out of range")
	}
	return parameters.Profile.Validate()
}

// Criteria are the predefined indicators a study compares groups against.
// A zero value means the indicator is not constrained.
type Criteria struct {
	MaxWasteSheets int     `json:"max_waste_sheets"`
	MaxMeanOffset  float64 `json:"max_mean_offset"`
	MaxMaxOffset   float64 `json:"max_max_offset"`
	MinSpeed       float64 `json:"min_speed"`
}

func (criteria Criteria) Validate() error {
	if criteria.MaxWasteSheets < 0 || criteria.MaxMeanOffset < 0 || criteria.MaxMaxOffset < 0 || criteria.MinSpeed < 0 {
		return fmt.Errorf("trial criteria must not be negative")
	}
	return nil
}

// Result records what one trial group actually produced: register offset
// samples in millimetres and the waste sheets counted during its run.
type Result struct {
	RegisterOffsets []float64 `json:"register_offsets"`
	WasteSheets     int       `json:"waste_sheets"`
	RecordedAt      time.Time `json:"recorded_at"`
}

func (result Result) Validate() error {
	if len(result.RegisterOffsets) == 0 {
		return fmt.Errorf("register offset samples are required")
	}
	for _, offset := range result.RegisterOffsets {
		if math.IsNaN(offset) || math.IsInf(offset, 0) || math.Abs(offset) > 50 {
			return fmt.Errorf("register offset sample is out of range")
		}
	}
	if result.WasteSheets < 0 {
		return fmt.Errorf("waste sheets must not be negative")
	}
	if result.RecordedAt.IsZero() {
		return fmt.Errorf("result recording time is required")
	}
	return nil
}

func (result Result) MeanOffset() float64 {
	total := 0.0
	for _, offset := range result.RegisterOffsets {
		total += math.Abs(offset)
	}
	return total / float64(len(result.RegisterOffsets))
}

func (result Result) MaxOffset() float64 {
	worst := 0.0
	for _, offset := range result.RegisterOffsets {
		if value := math.Abs(offset); value > worst {
			worst = value
		}
	}
	return worst
}

type Group struct {
	ID             string      `json:"id"`
	Kind           GroupKind   `json:"kind"`
	Label          string      `json:"label"`
	Status         GroupStatus `json:"status"`
	Parameters     Parameters  `json:"parameters"`
	Result         Result      `json:"result"`
	ResultRecorded bool        `json:"result_recorded"`
	CreatedAt      time.Time   `json:"created_at"`
}

// Study is one trial-print comparison for a single job edition (plate
// generation): an immutable baseline plus experiment groups, compared by
// predefined criteria. Groups are never deleted, so unadopted data is kept.
type Study struct {
	ID              string      `json:"id"`
	JobID           model.JobID `json:"job_id"`
	JobGeneration   uint64      `json:"job_generation"`
	PlateGeneration uint64      `json:"plate_generation"`
	Status          StudyStatus `json:"status"`
	Criteria        Criteria    `json:"criteria"`
	Groups          []Group     `json:"groups"`
	ApprovedGroupID string      `json:"approved_group_id"`
	AppliedAt       time.Time   `json:"applied_at"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

func NewStudy(job model.Job, criteria Criteria, baseline Parameters, now time.Time) (Study, error) {
	if err := job.Validate(); err != nil {
		return Study{}, err
	}
	if job.PlateGeneration == 0 {
		return Study{}, fmt.Errorf("study plate generation is required")
	}
	if err := criteria.Validate(); err != nil {
		return Study{}, err
	}
	if err := baseline.Validate(); err != nil {
		return Study{}, fmt.Errorf("baseline parameters: %w", err)
	}
	now = now.UTC()
	baselineGroup := Group{ID: uuid.NewString(), Kind: GroupBaseline, Label: "baseline", Status: GroupCollecting, Parameters: baseline, CreatedAt: now}
	return Study{ID: uuid.NewString(), JobID: job.ID, JobGeneration: job.Generation, PlateGeneration: job.PlateGeneration, Status: StudyOpen, Criteria: criteria, Groups: []Group{baselineGroup}, CreatedAt: now, UpdatedAt: now}, nil
}

func (study Study) Group(groupID string) (Group, bool) {
	for _, group := range study.Groups {
		if group.ID == groupID {
			return group, true
		}
	}
	return Group{}, false
}

func (study Study) Baseline() (Group, bool) {
	for _, group := range study.Groups {
		if group.Kind == GroupBaseline {
			return group, true
		}
	}
	return Group{}, false
}

func (study *Study) AddGroup(label string, parameters Parameters, now time.Time) (Group, error) {
	if study.Status != StudyOpen {
		return Group{}, fmt.Errorf("trial study is %s and no longer accepts groups", study.Status)
	}
	if label == "" {
		return Group{}, fmt.Errorf("trial group label is required")
	}
	if err := parameters.Validate(); err != nil {
		return Group{}, err
	}
	group := Group{ID: uuid.NewString(), Kind: GroupExperiment, Label: label, Status: GroupCollecting, Parameters: parameters, CreatedAt: now.UTC()}
	study.Groups = append(study.Groups, group)
	study.UpdatedAt = now.UTC()
	return group, nil
}

func (study *Study) RecordResult(groupID string, result Result, now time.Time) error {
	if study.Status != StudyOpen {
		return fmt.Errorf("trial study is %s and no longer accepts results", study.Status)
	}
	if err := result.Validate(); err != nil {
		return err
	}
	for index := range study.Groups {
		if study.Groups[index].ID == groupID {
			study.Groups[index].Result = result
			study.Groups[index].ResultRecorded = true
			study.Groups[index].Status = GroupComplete
			study.UpdatedAt = now.UTC()
			return nil
		}
	}
	return fmt.Errorf("trial group %q is not in the study", groupID)
}

func (study *Study) Approve(groupID string, now time.Time) error {
	if study.Status != StudyOpen {
		return fmt.Errorf("trial study is %s and cannot be approved", study.Status)
	}
	group, ok := study.Group(groupID)
	if !ok {
		return fmt.Errorf("trial group %q is not in the study", groupID)
	}
	evaluation := EvaluateGroup(group, study.Criteria)
	if !evaluation.Passed {
		return fmt.Errorf("trial group %q does not meet the study criteria: %v", groupID, evaluation.Violations)
	}
	study.Status = StudyApproved
	study.ApprovedGroupID = groupID
	study.UpdatedAt = now.UTC()
	return nil
}

// MarkApplied closes the study: the approved group becomes adopted and every
// other group is marked not adopted while its data is retained.
func (study *Study) MarkApplied(now time.Time) (Group, error) {
	if study.Status != StudyApproved {
		return Group{}, fmt.Errorf("trial study is %s and cannot be applied", study.Status)
	}
	var adopted Group
	for index := range study.Groups {
		if study.Groups[index].ID == study.ApprovedGroupID {
			study.Groups[index].Status = GroupAdopted
			adopted = study.Groups[index]
		} else {
			study.Groups[index].Status = GroupNotAdopted
		}
	}
	if adopted.ID == "" {
		return Group{}, fmt.Errorf("approved group %q is not in the study", study.ApprovedGroupID)
	}
	study.Status = StudyApplied
	study.AppliedAt = now.UTC()
	study.UpdatedAt = now.UTC()
	return adopted, nil
}
