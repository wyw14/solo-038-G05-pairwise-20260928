package trial

import (
	"fmt"
	"math"

	"github.com/wyw14/solo-038/internal/dryer"
)

// NewRegisterResult reduces raw register deviation samples (mm) into the
// stability summary stored with a result.
func NewRegisterResult(samples []float64) (RegisterResult, error) {
	if len(samples) == 0 {
		return RegisterResult{}, fmt.Errorf("register deviation samples are required")
	}
	result := RegisterResult{MaxDeviation: math.Abs(samples[0])}
	sum := 0.0
	for _, sample := range samples {
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			return RegisterResult{}, fmt.Errorf("register deviation sample is not finite")
		}
		deviation := math.Abs(sample)
		if deviation > result.MaxDeviation {
			result.MaxDeviation = deviation
		}
		sum += deviation
	}
	result.MeanDeviation = sum / float64(len(samples))
	variance := 0.0
	for _, sample := range samples {
		delta := math.Abs(sample) - result.MeanDeviation
		variance += delta * delta
	}
	result.StddevDeviation = math.Sqrt(variance / float64(len(samples)))
	return result, nil
}

func (result Result) validate() error {
	if result.Speed <= 0 || math.IsNaN(result.Speed) || math.IsInf(result.Speed, 0) {
		return fmt.Errorf("trial result speed must be positive")
	}
	if err := result.Profile.Validate(); err != nil {
		return err
	}
	if result.WasteSheets < 0 {
		return fmt.Errorf("trial result waste sheets cannot be negative")
	}
	if result.Register.MaxDeviation < 0 || result.Register.MeanDeviation < 0 || result.Register.StddevDeviation < 0 {
		return fmt.Errorf("trial result register deviations cannot be negative")
	}
	return nil
}

// DefaultProfile is the formal dryer curve used until a trial group is
// approved and put into production.
func DefaultProfile() dryer.Profile {
	return dryer.Profile{Name: "production", Zones: []dryer.Zone{{Name: "entry", Temperature: 80, Airflow: 20}, {Name: "exit", Temperature: 100, Airflow: 24}}}
}
