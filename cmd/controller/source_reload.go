package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"goetl/internal/persistence"
	"goetl/internal/reposource"
)

func (c *Controller) verifyActiveRunSources(ctx context.Context) error {
	if c.workflowStore == nil {
		return nil
	}
	runs, err := c.workflowStore.ListActiveWorkflowRuns(ctx)
	if err != nil {
		return fmt.Errorf("list active workflow runs: %w", err)
	}
	for _, run := range runs {
		if err := c.verifyWorkflowRunSource(ctx, run); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) verifyWorkflowRunSource(ctx context.Context, run persistence.WorkflowRunRecord) error {
	context, ok, err := workflowRunSourceContext(run)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	project, found, err := c.workflowStore.GetProject(ctx, run.ProjectID)
	if err != nil {
		return fmt.Errorf("reload run %s project %s: %w", run.ID, run.ProjectID, err)
	}
	if !found {
		return fmt.Errorf("reload run %s project %s was not found", run.ID, run.ProjectID)
	}
	workflow, found, err := c.workflowStore.GetWorkflow(ctx, run.WorkflowID)
	if err != nil {
		return fmt.Errorf("reload run %s workflow %s: %w", run.ID, run.WorkflowID, err)
	}
	if !found {
		return fmt.Errorf("reload run %s workflow %s was not found", run.ID, run.WorkflowID)
	}

	manifest, err := readAdmittedSourceManifest(context.SourceAdmission.ManifestRef)
	if err != nil {
		return fmt.Errorf("reload run %s source %s: %w", run.ID, context.SourceAdmission.Source.RepositoryIdentity, err)
	}
	if err := c.verifyCachedRunSource(project, workflow, manifest); err == nil {
		return nil
	} else if !isRepairableGitHubSource(manifest.Source) {
		return fmt.Errorf("reload run %s source %s: local source cache cannot be repaired: %w", run.ID, manifest.Source.Repository.Value, err)
	}

	if err := c.repairGitHubRunSource(ctx, manifest); err != nil {
		return fmt.Errorf("reload run %s source %s repair: %w", run.ID, manifest.Source.Repository.Value, err)
	}
	if err := c.verifyCachedRunSource(project, workflow, manifest); err != nil {
		return fmt.Errorf("reload run %s source %s repaired cache failed verification: %w", run.ID, manifest.Source.Repository.Value, err)
	}
	return nil
}

func workflowRunSourceContext(run persistence.WorkflowRunRecord) (workflowRunSubmissionContext, bool, error) {
	var context workflowRunSubmissionContext
	if err := json.Unmarshal([]byte(run.SubmissionContextJSON), &context); err != nil {
		return workflowRunSubmissionContext{}, false, fmt.Errorf("reload run %s submission context: %w", run.ID, err)
	}
	if context.Schema != workflowRunSubmissionContextSchemaV1 || context.SourceAdmission.ManifestRef == "" {
		return workflowRunSubmissionContext{}, false, nil
	}
	return context, true, nil
}

func readAdmittedSourceManifest(path string) (reposource.AdmittedSourceManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return reposource.AdmittedSourceManifest{}, fmt.Errorf("read admitted source manifest: %w", err)
	}
	var manifest reposource.AdmittedSourceManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return reposource.AdmittedSourceManifest{}, fmt.Errorf("decode admitted source manifest: %w", err)
	}
	return manifest, nil
}

func (c *Controller) verifyCachedRunSource(project persistence.ProjectRecord, workflow persistence.WorkflowRecord, manifest reposource.AdmittedSourceManifest) error {
	access, err := reposource.NewCacheAccess(c.repoCacheLayout, manifest)
	if err != nil {
		return err
	}
	projectFile, err := manifestFileByRole(manifest, reposource.FileRoleProject)
	if err != nil {
		return err
	}
	workflowFile, err := manifestFileByRole(manifest, reposource.FileRoleWorkflow)
	if err != nil {
		return err
	}
	if err := verifyCachedJSONHash(access, projectFile, project.ConfigSHA256); err != nil {
		return fmt.Errorf("project %s: %w", project.ConfigPath, err)
	}
	if err := verifyCachedJSONHash(access, workflowFile, workflow.WorkflowSHA256); err != nil {
		return fmt.Errorf("workflow %s: %w", workflow.WorkflowPath, err)
	}
	return nil
}

func verifyCachedJSONHash(access reposource.CacheAccess, file reposource.AdmittedSourceManifestFile, want string) error {
	data, err := access.ReadFile(file.CachePath)
	if err != nil {
		return err
	}
	_, got, err := canonicalSourceDocument(data)
	if err != nil {
		return fmt.Errorf("canonical json: %w", err)
	}
	if got != want {
		return fmt.Errorf("canonical sha256 %s != %s", got, want)
	}
	return nil
}

func isRepairableGitHubSource(source reposource.ResolvedSourceReference) bool {
	return source.RevisionID != nil && *source.RevisionID != "" && strings.HasPrefix(source.Repository.Value, "github.com/")
}

func (c *Controller) repairGitHubRunSource(ctx context.Context, manifest reposource.AdmittedSourceManifest) error {
	provider, err := c.repositorySourceProvider(SourceDocumentReference{
		Repository: manifest.Source.Repository.Value,
		Ref:        sourceRevisionIDValue(manifest.Source.RevisionID),
		Path:       "project.json",
	})
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		paths = append(paths, file.SourcePath)
	}
	reads, err := provider.ReadFiles(ctx, manifest.Source, paths)
	if err != nil {
		return err
	}
	return reposource.PublishAdmittedSource(c.repoCacheLayout, manifest, reads)
}
