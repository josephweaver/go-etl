package model

import (
	"math"
	"strings"
	"testing"
)

func TestResourceConstraintAllows(t *testing.T) {
	tests := []struct {
		name           string
		totalUnits     int64
		requestedUnits int64
		operator       ResourceOperator
		targetUnits    int64
		want           bool
	}{
		{name: "equal true", totalUnits: 2, requestedUnits: 3, operator: ResourceOperatorEqual, targetUnits: 5, want: true},
		{name: "equal false", totalUnits: 2, requestedUnits: 3, operator: ResourceOperatorEqual, targetUnits: 6},
		{name: "not equal true", totalUnits: 2, requestedUnits: 3, operator: ResourceOperatorNotEqual, targetUnits: 6, want: true},
		{name: "not equal false", totalUnits: 2, requestedUnits: 3, operator: ResourceOperatorNotEqual, targetUnits: 5},
		{name: "less than true", totalUnits: 2, requestedUnits: 3, operator: ResourceOperatorLessThan, targetUnits: 6, want: true},
		{name: "less than false equal", totalUnits: 2, requestedUnits: 3, operator: ResourceOperatorLessThan, targetUnits: 5},
		{name: "greater than true", totalUnits: 2, requestedUnits: 3, operator: ResourceOperatorGreaterThan, targetUnits: 4, want: true},
		{name: "greater than false equal", totalUnits: 2, requestedUnits: 3, operator: ResourceOperatorGreaterThan, targetUnits: 5},
		{name: "less than or equal true less", totalUnits: 2, requestedUnits: 3, operator: ResourceOperatorLessThanOrEqual, targetUnits: 6, want: true},
		{name: "less than or equal true equal", totalUnits: 2, requestedUnits: 3, operator: ResourceOperatorLessThanOrEqual, targetUnits: 5, want: true},
		{name: "less than or equal false", totalUnits: 2, requestedUnits: 3, operator: ResourceOperatorLessThanOrEqual, targetUnits: 4},
		{name: "greater than or equal true greater", totalUnits: 2, requestedUnits: 3, operator: ResourceOperatorGreaterThanOrEqual, targetUnits: 4, want: true},
		{name: "greater than or equal true equal", totalUnits: 2, requestedUnits: 3, operator: ResourceOperatorGreaterThanOrEqual, targetUnits: 5, want: true},
		{name: "greater than or equal false", totalUnits: 2, requestedUnits: 3, operator: ResourceOperatorGreaterThanOrEqual, targetUnits: 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResourceConstraintAllows(tt.totalUnits, tt.requestedUnits, tt.operator, tt.targetUnits)
			if err != nil {
				t.Fatalf("ResourceConstraintAllows() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResourceConstraintAllows() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResourceConstraintAllowsRejectsOverflow(t *testing.T) {
	allowed, err := ResourceConstraintAllows(math.MaxInt64, 1, ResourceOperatorLessThanOrEqual, math.MaxInt64)
	if err == nil || !strings.Contains(err.Error(), "resource units overflow") {
		t.Fatalf("ResourceConstraintAllows() error = %v, want overflow", err)
	}
	if allowed {
		t.Fatal("ResourceConstraintAllows() allowed = true, want false")
	}
}

func TestResourceConstraintAllowsRejectsUnsupportedOperator(t *testing.T) {
	allowed, err := ResourceConstraintAllows(1, 1, ResourceOperator("approximately"), 2)
	if err == nil || !strings.Contains(err.Error(), "unsupported resource operator") {
		t.Fatalf("ResourceConstraintAllows() error = %v, want unsupported operator", err)
	}
	if allowed {
		t.Fatal("ResourceConstraintAllows() allowed = true, want false")
	}
}

func TestResourceConstraintChecksAllow(t *testing.T) {
	tests := []struct {
		name   string
		checks []ResourceConstraintCheck
		want   bool
	}{
		{name: "zero checks is eligible", want: true},
		{
			name: "all checks pass",
			checks: []ResourceConstraintCheck{
				{TotalUnits: 1, RequestedUnits: 1, Operator: ResourceOperatorLessThanOrEqual, TargetUnits: 2},
				{TotalUnits: 0, RequestedUnits: 1, Operator: ResourceOperatorGreaterThanOrEqual, TargetUnits: 1},
			},
			want: true,
		},
		{
			name: "one check fails",
			checks: []ResourceConstraintCheck{
				{TotalUnits: 1, RequestedUnits: 1, Operator: ResourceOperatorLessThanOrEqual, TargetUnits: 2},
				{TotalUnits: 2, RequestedUnits: 1, Operator: ResourceOperatorLessThan, TargetUnits: 3},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResourceConstraintChecksAllow(tt.checks)
			if err != nil {
				t.Fatalf("ResourceConstraintChecksAllow() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResourceConstraintChecksAllow() = %v, want %v", got, tt.want)
			}
		})
	}
}
