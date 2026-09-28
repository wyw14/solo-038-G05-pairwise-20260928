package trial

import (
	"fmt"
	"sort"
	"time"
)

type Evaluation struct {
	GroupID        string    `json:"group_id"`
	Kind           GroupKind `json:"kind"`
	Label          string    `json:"label"`
	Passed         bool      `json:"passed"`
	Violations     []string  `json:"violations"`
	MeanOffset     float64   `json:"mean_offset"`
	MaxOffset      float64   `json:"max_offset"`
	WasteSheets    int       `json:"waste_sheets"`
	Speed          float64   `json:"speed"`
	ResultRecorded bool      `json:"result_recorded"`
}

func EvaluateGroup(group Group, criteria Criteria) Evaluation {
	evaluation := Evaluation{GroupID: group.ID, Kind: group.Kind, Label: group.Label, Speed: group.Parameters.Speed, ResultRecorded: group.ResultRecorded}
	if !group.ResultRecorded {
		evaluation.Violations = []string{"result not recorded"}
		return evaluation
	}
	evaluation.WasteSheets = group.Result.WasteSheets
	evaluation.MeanOffset = group.Result.MeanOffset()
	evaluation.MaxOffset = group.Result.MaxOffset()
	if criteria.MaxWasteSheets > 0 && evaluation.WasteSheets > criteria.MaxWasteSheets {
		evaluation.Violations = append(evaluation.Violations, fmt.Sprintf("waste sheets %d exceed limit %d", evaluation.WasteSheets, criteria.MaxWasteSheets))
	}
	if criteria.MaxMeanOffset > 0 && evaluation.MeanOffset > criteria.MaxMeanOffset {
		evaluation.Violations = append(evaluation.Violations, fmt.Sprintf("mean register offset %.3f exceeds limit %.3f", evaluation.MeanOffset, criteria.MaxMeanOffset))
	}
	if criteria.MaxMaxOffset > 0 && evaluation.MaxOffset > criteria.MaxMaxOffset {
		evaluation.Violations = append(evaluation.Violations, fmt.Sprintf("max register offset %.3f exceeds limit %.3f", evaluation.MaxOffset, criteria.MaxMaxOffset))
	}
	if criteria.MinSpeed > 0 && evaluation.Speed < criteria.MinSpeed {
		evaluation.Violations = append(evaluation.Violations, fmt.Sprintf("speed %.2f is below minimum %.2f", evaluation.Speed, criteria.MinSpeed))
	}
	evaluation.Passed = len(evaluation.Violations) == 0
	return evaluation
}

type Comparison struct {
	StudyID            string       `json:"study_id"`
	BaselineGroupID    string       `json:"baseline_group_id"`
	RecommendedGroupID string       `json:"recommended_group_id"`
	Evaluations        []Evaluation `json:"evaluations"`
	PendingGroupIDs    []string     `json:"pending_group_ids"`
	EvaluatedAt        time.Time    `json:"evaluated_at"`
}

// Compare ranks every group with a recorded result against the study
// criteria: passing groups first, then fewest waste sheets, then the most
// stable register, then the highest speed. Ties break on creation order so
// the ranking is deterministic.
func Compare(study Study, now time.Time) Comparison {
	comparison := Comparison{StudyID: study.ID, Evaluations: []Evaluation{}, PendingGroupIDs: []string{}, EvaluatedAt: now.UTC()}
	if baseline, ok := study.Baseline(); ok {
		comparison.BaselineGroupID = baseline.ID
	}
	created := map[string]time.Time{}
	for _, group := range study.Groups {
		created[group.ID] = group.CreatedAt
		if !group.ResultRecorded {
			comparison.PendingGroupIDs = append(comparison.PendingGroupIDs, group.ID)
			continue
		}
		comparison.Evaluations = append(comparison.Evaluations, EvaluateGroup(group, study.Criteria))
	}
	sort.SliceStable(comparison.Evaluations, func(left, right int) bool {
		a, b := comparison.Evaluations[left], comparison.Evaluations[right]
		if a.Passed != b.Passed {
			return a.Passed
		}
		if a.WasteSheets != b.WasteSheets {
			return a.WasteSheets < b.WasteSheets
		}
		if a.MeanOffset != b.MeanOffset {
			return a.MeanOffset < b.MeanOffset
		}
		if a.Speed != b.Speed {
			return a.Speed > b.Speed
		}
		if !created[a.GroupID].Equal(created[b.GroupID]) {
			return created[a.GroupID].Before(created[b.GroupID])
		}
		return a.GroupID < b.GroupID
	})
	for _, evaluation := range comparison.Evaluations {
		if evaluation.Passed {
			comparison.RecommendedGroupID = evaluation.GroupID
			break
		}
	}
	return comparison
}
