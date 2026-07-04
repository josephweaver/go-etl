package reposource

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// GitHubProvider reads declared files from one GitHub repository through the GitHub REST API.
type GitHubProvider struct {
	repository RepositoryIdentity
	owner      string
	repo       string
	baseURL    string
	client     *http.Client
	token      string
}

type GitHubProviderOption func(*GitHubProvider)

func NewGitHubProvider(owner string, repo string, opts ...GitHubProviderOption) GitHubProvider {
	provider := GitHubProvider{
		repository: RepositoryIdentity{
			Value:       "github.com/" + owner + "/" + repo,
			DisplayName: owner + "/" + repo,
		},
		owner:   owner,
		repo:    repo,
		baseURL: "https://api.github.com",
		client:  http.DefaultClient,
	}
	for _, opt := range opts {
		opt(&provider)
	}
	if provider.client == nil {
		provider.client = http.DefaultClient
	}
	provider.baseURL = strings.TrimRight(provider.baseURL, "/")
	return provider
}

func WithGitHubBaseURL(baseURL string) GitHubProviderOption {
	return func(p *GitHubProvider) {
		p.baseURL = baseURL
	}
}

func WithGitHubHTTPClient(client *http.Client) GitHubProviderOption {
	return func(p *GitHubProvider) {
		p.client = client
	}
}

func WithGitHubToken(token string) GitHubProviderOption {
	return func(p *GitHubProvider) {
		p.token = token
	}
}

func (p GitHubProvider) Resolve(ctx context.Context, requestedRef string) (ResolvedSourceReference, error) {
	if requestedRef == "" {
		return ResolvedSourceReference{}, fmt.Errorf("github ref is required")
	}
	var response struct {
		SHA string `json:"sha"`
	}
	apiPath := fmt.Sprintf("/repos/%s/%s/commits/%s", url.PathEscape(p.owner), url.PathEscape(p.repo), url.PathEscape(requestedRef))
	if err := p.getJSON(ctx, apiPath, &response); err != nil {
		return ResolvedSourceReference{}, fmt.Errorf("resolve github ref %s: %w", requestedRef, err)
	}
	if response.SHA == "" {
		return ResolvedSourceReference{}, fmt.Errorf("resolve github ref %s: response missing sha", requestedRef)
	}
	return ResolvedSourceReference{
		Repository:   p.repository,
		RequestedRef: requestedRef,
		RevisionID:   &response.SHA,
	}, nil
}

func (p GitHubProvider) ReadFiles(ctx context.Context, resolved ResolvedSourceReference, paths []string) ([]ReadFileResult, error) {
	if resolved.RevisionID == nil || *resolved.RevisionID == "" {
		return nil, fmt.Errorf("github revision id is required")
	}
	requests, err := sourceFileRequests(resolved.Repository, resolved.RevisionID, paths)
	if err != nil {
		return nil, err
	}

	results := make([]ReadFileResult, 0, len(requests))
	for _, request := range requests {
		var response struct {
			SHA      string `json:"sha"`
			Type     string `json:"type"`
			Encoding string `json:"encoding"`
			Content  string `json:"content"`
		}
		apiPath := fmt.Sprintf(
			"/repos/%s/%s/contents/%s?ref=%s",
			url.PathEscape(p.owner),
			url.PathEscape(p.repo),
			escapePathSegments(request.SourcePath),
			url.QueryEscape(*resolved.RevisionID),
		)
		if err := p.getJSON(ctx, apiPath, &response); err != nil {
			return nil, fmt.Errorf("read github source file %s: %w", request.SourcePath, err)
		}
		if response.Type != "" && response.Type != "file" {
			return nil, fmt.Errorf("read github source file %s: response is %s, not file", request.SourcePath, response.Type)
		}
		if response.Encoding != "base64" {
			return nil, fmt.Errorf("read github source file %s: unsupported encoding %q", request.SourcePath, response.Encoding)
		}
		decoded, err := base64.StdEncoding.DecodeString(stripBase64Whitespace(response.Content))
		if err != nil {
			return nil, fmt.Errorf("decode github source file %s: %w", request.SourcePath, err)
		}
		objectID := response.SHA
		results = append(results, newReadFileResult(request, decoded, &objectID))
	}
	return results, nil
}

func (p GitHubProvider) getJSON(ctx context.Context, apiPath string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+apiPath, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("github api status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode github api response: %w", err)
	}
	return nil
}

func escapePathSegments(value string) string {
	segments := strings.Split(value, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return path.Join(segments...)
}

func stripBase64Whitespace(value string) string {
	return strings.NewReplacer("\n", "", "\r", "", "\t", "", " ", "").Replace(value)
}
