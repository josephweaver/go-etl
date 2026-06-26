package main

import "context"

type Preparer interface {
	Prepare(ctx context.Context) error
}

const (
	PreflightSeverityError   = "error"
	PreflightSeverityWarning = "warning"
)

type PreflightIssue struct {
	Component   string `json:"component"`
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

type PreflightComponent interface {
	Preflight(ctx context.Context) []PreflightIssue
}

func prepareIfSupported(ctx context.Context, value any) error {
	preparer, ok := value.(Preparer)
	if !ok {
		return nil
	}
	return preparer.Prepare(ctx)
}

func preflightIfSupported(ctx context.Context, value any) []PreflightIssue {
	component, ok := value.(PreflightComponent)
	if !ok {
		return nil
	}
	return component.Preflight(ctx)
}

func blockingPreflightIssues(issues []PreflightIssue) []PreflightIssue {
	blocking := make([]PreflightIssue, 0, len(issues))
	for _, issue := range issues {
		if issue.Severity == PreflightSeverityError {
			blocking = append(blocking, issue)
		}
	}
	return blocking
}
