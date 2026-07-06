package model

import (
	"fmt"
	"math"
)

type ResourceOperator = WorkItemResourceConstraintOperator

const (
	ResourceOperatorEqual              ResourceOperator = WorkItemResourceConstraintOperatorEqual
	ResourceOperatorNotEqual           ResourceOperator = WorkItemResourceConstraintOperatorNotEqual
	ResourceOperatorLessThan           ResourceOperator = WorkItemResourceConstraintOperatorLessThan
	ResourceOperatorGreaterThan        ResourceOperator = WorkItemResourceConstraintOperatorGreater
	ResourceOperatorLessThanOrEqual    ResourceOperator = WorkItemResourceConstraintOperatorLessEq
	ResourceOperatorGreaterThanOrEqual ResourceOperator = WorkItemResourceConstraintOperatorGreaterEq
)

type ResourceConstraintCheck struct {
	TotalUnits     int64
	RequestedUnits int64
	Operator       ResourceOperator
	TargetUnits    int64
}

func ResourceConstraintAllows(totalUnits int64, requestedUnits int64, operator ResourceOperator, targetUnits int64) (bool, error) {
	candidateTotal, err := checkedAdd(totalUnits, requestedUnits)
	if err != nil {
		return false, err
	}

	switch operator {
	case ResourceOperatorEqual:
		return candidateTotal == targetUnits, nil
	case ResourceOperatorNotEqual:
		return candidateTotal != targetUnits, nil
	case ResourceOperatorLessThan:
		return candidateTotal < targetUnits, nil
	case ResourceOperatorGreaterThan:
		return candidateTotal > targetUnits, nil
	case ResourceOperatorLessThanOrEqual:
		return candidateTotal <= targetUnits, nil
	case ResourceOperatorGreaterThanOrEqual:
		return candidateTotal >= targetUnits, nil
	default:
		return false, fmt.Errorf("unsupported resource operator %q", operator)
	}
}

func ResourceConstraintChecksAllow(checks []ResourceConstraintCheck) (bool, error) {
	for index, check := range checks {
		allowed, err := ResourceConstraintAllows(check.TotalUnits, check.RequestedUnits, check.Operator, check.TargetUnits)
		if err != nil {
			return false, fmt.Errorf("resource constraint check %d: %w", index, err)
		}
		if !allowed {
			return false, nil
		}
	}
	return true, nil
}

func checkedAdd(left int64, right int64) (int64, error) {
	if (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right) {
		return 0, fmt.Errorf("resource units overflow: %d + %d", left, right)
	}
	return left + right, nil
}
