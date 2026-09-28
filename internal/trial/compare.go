package trial

import (
	"fmt"
	"sort"

	"github.com/wyw14/solo-038/internal/model"
)

// Criteria are the predetermined metrics a trial is judged by. Limits gate
// eligibility (zero disables a limit); weights steer the ranking score.
type Criteria struct {
	MaxWasteSheets       int     `json:"max_waste_sheets"`
	MaxRegisterDeviation float64 `json:"max_register_deviation"`
	MinSpeed             float64 `json:"min_speed"`
	WeightWaste          float64 `json:"weight_waste"`
	WeightRegister       float64 `json:"weight_register"`
	WeightSpeed          float64 `json:"weight_speed"`
}

func DefaultCriteria() Criteria {
	return Criteria{WeightWaste: 0.45, WeightRegister: 0.45, WeightSpeed: 0.10}
}

func (criteria Criteria) withDefaults() Criteria {
	defaults := DefaultCriteria()
	if criteria.WeightWaste == 0 && criteria.WeightRegister == 0 && criteria.WeightSpeed == 0 {
		criteria.WeightWaste, criteria.WeightRegister, criteria.WeightSpeed = defaults.WeightWaste, defaults.WeightRegister, defaults.WeightSpeed
	}
	return criteria
}

func (criteria Criteria) Validate() error {
	if criteria.MaxWasteSheets < 0 || criteria.MaxRegisterDeviation < 0 || criteria.MinSpeed < 0 {
		return fmt.Errorf("trial criteria limits cannot be negative")
	}
	if criteria.WeightWaste < 0 || criteria.WeightRegister < 0 || criteria.WeightSpeed < 0 {
		return fmt.Errorf("trial criteria weights cannot be negative")
	}
	if criteria.WeightWaste+criteria.WeightRegister+criteria.WeightSpeed <= 0 {
		return fmt.Errorf("trial criteria weights are required")
	}
	return nil
}

// GroupScore aggregates one group's recorded runs against the criteria.
type GroupScore struct {
	GroupID                 model.GroupID `json:"group_id"`
	Name                    string        `json:"name"`
	Kind                    GroupKind     `json:"kind"`
	Status                  GroupStatus   `json:"status"`
	Runs                    int           `json:"runs"`
	TotalWasteSheets        int           `json:"total_waste_sheets"`
	AverageSpeed            float64       `json:"average_speed"`
	MaxRegisterDeviation    float64       `json:"max_register_deviation"`
	MeanRegisterDeviation   float64       `json:"mean_register_deviation"`
	StddevRegisterDeviation float64       `json:"stddev_register_deviation"`
	Eligible                bool          `json:"eligible"`
	Violations              []string      `json:"violations,omitempty"`
	Score                   float64       `json:"score"`
	Rank                    int           `json:"rank"`
}

// Report compares every recorded group of one trial against its criteria.
type Report struct {
	TrialID          model.TrialID `json:"trial_id"`
	JobID            model.JobID   `json:"job_id"`
	PlateGeneration  uint64        `json:"plate_generation"`
	Criteria         Criteria      `json:"criteria"`
	Groups           []GroupScore  `json:"groups"`
	RecommendedGroup model.GroupID `json:"recommended_group,omitempty"`
}

func summarize(group Group) GroupScore {
	score := GroupScore{GroupID: group.ID, Name: group.Name, Kind: group.Kind, Status: group.Status, Runs: len(group.Results)}
	for _, result := range group.Results {
		score.TotalWasteSheets += result.WasteSheets
		score.AverageSpeed += result.Speed
		if result.Register.MaxDeviation > score.MaxRegisterDeviation {
			score.MaxRegisterDeviation = result.Register.MaxDeviation
		}
		score.MeanRegisterDeviation += result.Register.MeanDeviation
		score.StddevRegisterDeviation += result.Register.StddevDeviation
	}
	if score.Runs > 0 {
		score.AverageSpeed /= float64(score.Runs)
		score.MeanRegisterDeviation /= float64(score.Runs)
		score.StddevRegisterDeviation /= float64(score.Runs)
	}
	return score
}

func (criteria Criteria) check(score GroupScore) GroupScore {
	violations := []string{}
	if criteria.MaxWasteSheets > 0 && score.TotalWasteSheets > criteria.MaxWasteSheets {
		violations = append(violations, fmt.Sprintf("waste sheets %d exceed limit %d", score.TotalWasteSheets, criteria.MaxWasteSheets))
	}
	if criteria.MaxRegisterDeviation > 0 && score.MaxRegisterDeviation > criteria.MaxRegisterDeviation {
		violations = append(violations, fmt.Sprintf("register deviation %.3f exceeds limit %.3f", score.MaxRegisterDeviation, criteria.MaxRegisterDeviation))
	}
	if criteria.MinSpeed > 0 && score.AverageSpeed < criteria.MinSpeed {
		violations = append(violations, fmt.Sprintf("average speed %.3f below minimum %.3f", score.AverageSpeed, criteria.MinSpeed))
	}
	score.Violations = violations
	score.Eligible = len(violations) == 0
	return score
}

// Compare ranks every group that has recorded results. Eligible groups are
// scored by weighted normalised metrics; ineligible groups rank below them.
func Compare(trial Trial) (Report, error) {
	if err := trial.Criteria.Validate(); err != nil {
		return Report{}, err
	}
	report := Report{TrialID: trial.ID, JobID: trial.JobID, PlateGeneration: trial.PlateGeneration, Criteria: trial.Criteria}
	for _, group := range trial.Groups {
		if len(group.Results) == 0 {
			continue
		}
		report.Groups = append(report.Groups, trial.Criteria.check(summarize(group)))
	}
	if len(report.Groups) < 2 {
		return Report{}, ErrNotEnoughGroups
	}
	eligible := []GroupScore{}
	for _, score := range report.Groups {
		if score.Eligible {
			eligible = append(eligible, score)
		}
	}
	if len(eligible) > 0 {
		minWaste, maxWaste := eligible[0].TotalWasteSheets, eligible[0].TotalWasteSheets
		minDeviation, maxDeviation := eligible[0].MaxRegisterDeviation, eligible[0].MaxRegisterDeviation
		minSpeed, maxSpeed := eligible[0].AverageSpeed, eligible[0].AverageSpeed
		for _, score := range eligible[1:] {
			minWaste = min(minWaste, score.TotalWasteSheets)
			maxWaste = max(maxWaste, score.TotalWasteSheets)
			minDeviation = min(minDeviation, score.MaxRegisterDeviation)
			maxDeviation = max(maxDeviation, score.MaxRegisterDeviation)
			minSpeed = min(minSpeed, score.AverageSpeed)
			maxSpeed = max(maxSpeed, score.AverageSpeed)
		}
		weightTotal := trial.Criteria.WeightWaste + trial.Criteria.WeightRegister + trial.Criteria.WeightSpeed
		for index, score := range report.Groups {
			if !score.Eligible {
				continue
			}
			wasteScore := lowerIsBetter(float64(score.TotalWasteSheets), float64(minWaste), float64(maxWaste))
			registerScore := lowerIsBetter(score.MaxRegisterDeviation, minDeviation, maxDeviation)
			speedScore := higherIsBetter(score.AverageSpeed, minSpeed, maxSpeed)
			report.Groups[index].Score = (trial.Criteria.WeightWaste*wasteScore + trial.Criteria.WeightRegister*registerScore + trial.Criteria.WeightSpeed*speedScore) / weightTotal
		}
	}
	sort.SliceStable(report.Groups, func(left, right int) bool {
		if report.Groups[left].Eligible != report.Groups[right].Eligible {
			return report.Groups[left].Eligible
		}
		if report.Groups[left].Score != report.Groups[right].Score {
			return report.Groups[left].Score > report.Groups[right].Score
		}
		if report.Groups[left].TotalWasteSheets != report.Groups[right].TotalWasteSheets {
			return report.Groups[left].TotalWasteSheets < report.Groups[right].TotalWasteSheets
		}
		if report.Groups[left].MaxRegisterDeviation != report.Groups[right].MaxRegisterDeviation {
			return report.Groups[left].MaxRegisterDeviation < report.Groups[right].MaxRegisterDeviation
		}
		return report.Groups[left].Name < report.Groups[right].Name
	})
	for index := range report.Groups {
		report.Groups[index].Rank = index + 1
	}
	for _, score := range report.Groups {
		if score.Eligible {
			report.RecommendedGroup = score.GroupID
			break
		}
	}
	return report, nil
}

func lowerIsBetter(value, low, high float64) float64 {
	if high <= low {
		return 1
	}
	return (high - value) / (high - low)
}

func higherIsBetter(value, low, high float64) float64 {
	if high <= low {
		return 1
	}
	return (value - low) / (high - low)
}
