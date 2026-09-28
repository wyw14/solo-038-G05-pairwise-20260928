package model

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type OperationID string
type JobID string
type LeaseID string
type EventID string
type TrialID string
type GroupID string

func NewOperationID() OperationID {
	return OperationID(uuid.NewString())
}

func NewJobID() JobID {
	return JobID(uuid.NewString())
}

func NewLeaseID() LeaseID {
	return LeaseID(uuid.NewString())
}

func NewEventID() EventID {
	return EventID(uuid.NewString())
}

func NewTrialID() TrialID {
	return TrialID(uuid.NewString())
}

func NewGroupID() GroupID {
	return GroupID(uuid.NewString())
}

func ParseJobID(value string) (JobID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("job id is required")
	}
	if _, err := uuid.Parse(value); err != nil {
		return "", fmt.Errorf("invalid job id: %w", err)
	}
	return JobID(value), nil
}

func ParseTrialID(value string) (TrialID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("trial id is required")
	}
	if _, err := uuid.Parse(value); err != nil {
		return "", fmt.Errorf("invalid trial id: %w", err)
	}
	return TrialID(value), nil
}

func ParseGroupID(value string) (GroupID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("group id is required")
	}
	if _, err := uuid.Parse(value); err != nil {
		return "", fmt.Errorf("invalid group id: %w", err)
	}
	return GroupID(value), nil
}

func (id OperationID) Valid() bool {
	_, err := uuid.Parse(string(id))
	return err == nil
}

func (id JobID) Valid() bool {
	_, err := uuid.Parse(string(id))
	return err == nil
}

func (id LeaseID) Valid() bool {
	_, err := uuid.Parse(string(id))
	return err == nil
}

func (id TrialID) Valid() bool {
	_, err := uuid.Parse(string(id))
	return err == nil
}

func (id GroupID) Valid() bool {
	_, err := uuid.Parse(string(id))
	return err == nil
}
