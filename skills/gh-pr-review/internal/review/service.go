package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/agynio/gh-pr-review/internal/ghcli"
	"github.com/agynio/gh-pr-review/internal/resolver"
)

// Service coordinates review GraphQL operations through the gh CLI.
type Service struct {
	API ghcli.API
}

// ErrViewerLoginUnavailable indicates the authenticated viewer login could not be resolved via GraphQL.
var ErrViewerLoginUnavailable = errors.New("viewer login unavailable")

// ReviewState contains metadata about a review after opening or submitting it.
type ReviewState struct {
	ID          string  `json:"id"`
	State       string  `json:"state"`
	SubmittedAt *string `json:"submitted_at,omitempty"`
}

// SubmitStatus represents the outcome of a review submission mutation.
type SubmitStatus struct {
	Success bool
	Errors  []ghcli.GraphQLErrorEntry
}

// ReviewThread represents an inline comment thread added to a pending review.
type ReviewThread struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	IsOutdated bool   `json:"is_outdated"`
	Line       *int   `json:"line,omitempty"`
}

// ThreadInput describes the inline comment details for AddThread.
type ThreadInput struct {
	ReviewID  string
	Path      string
	Line      int
	Side      string
	StartLine *int
	StartSide *string
	Body      string
}

// SubmitInput contains the payload for submitting a pending review.
type SubmitInput struct {
	ReviewID string
	Event    string
	Body     string
}

// NewService constructs a review Service.
func NewService(api ghcli.API) *Service {
	return &Service{API: api}
}

// Start opens a pending review for the specified pull request.
func (s *Service) Start(pr resolver.Identity, commitOID string) (*ReviewState, error) {
	nodeID, headSHA, err := s.pullRequestIdentifiers(pr)
	if err != nil {
		return nil, err
	}

	trimmedCommit := strings.TrimSpace(commitOID)
	if trimmedCommit == "" {
		trimmedCommit = headSHA
	}

	const mutation = `mutation($input:AddPullRequestReviewInput!){
  addPullRequestReview(input:$input){
    pullRequestReview { id state submittedAt }
  }
}`

	payload := map[string]interface{}{
		"input": map[string]interface{}{
			"pullRequestId": nodeID,
			"commitOID":     trimmedCommit,
		},
	}

	var resp struct {
		AddPullRequestReview struct {
			PullRequestReview struct {
				ID          string  `json:"id"`
				State       string  `json:"state"`
				SubmittedAt *string `json:"submittedAt"`
			} `json:"pullRequestReview"`
		} `json:"addPullRequestReview"`
	}

	if err := s.API.GraphQL(mutation, payload, &resp); err != nil {
		return nil, err
	}

	prr := resp.AddPullRequestReview.PullRequestReview
	trimmedID := strings.TrimSpace(prr.ID)
	if trimmedID == "" {
		return nil, errors.New("addPullRequestReview returned empty id")
	}
	trimmedState := strings.TrimSpace(prr.State)
	if trimmedState == "" {
		return nil, errors.New("addPullRequestReview returned empty state")
	}
	state := ReviewState{ID: trimmedID, State: trimmedState}

	if prr.SubmittedAt != nil {
		trimmed := strings.TrimSpace(*prr.SubmittedAt)
		if trimmed != "" {
			state.SubmittedAt = &trimmed
		}
	}

	return &state, nil
}

func (s *Service) ensureNoExistingCommentAt(pr resolver.Identity, path string, line int) error {
	viewer, err := s.currentViewer()
	if err != nil {
		return err
	}

	// Fetch reviewThreads using gh pr view --json
	args := []string{"pr", "view", strconv.Itoa(pr.Number), "--json", "reviewThreads"}
	stdout, _, err := s.API.Exec(args...)
	if err != nil {
		return fmt.Errorf("checking existing comments: %w", err)
	}

	var data struct {
		ReviewThreads []struct {
			Path     string `json:"path"`
			Line     int    `json:"line"`
			Comments []struct {
				Author struct {
					Login string `json:"login"`
				} `json:"author"`
			} `json:"comments"`
		} `json:"reviewThreads"`
	}

	if err := json.Unmarshal(stdout, &data); err != nil {
		return fmt.Errorf("parsing pr view output: %w", err)
	}

	for _, t := range data.ReviewThreads {
		if t.Path == path && t.Line == line {
			for _, c := range t.Comments {
				if strings.EqualFold(c.Author.Login, viewer) {
					return fmt.Errorf("an inline comment by %q already exists on %s:%d; skipping duplicate comment", viewer, path, line)
				}
			}
		}
	}

	return nil
}

// AddThread adds an inline review comment thread to an existing pending review.
func (s *Service) AddThread(pr resolver.Identity, input ThreadInput) (*ReviewThread, error) {
	if err := s.ensureNoExistingCommentAt(pr, strings.TrimSpace(input.Path), input.Line); err != nil {
		return nil, err
	}

	trimmedID := strings.TrimSpace(input.ReviewID)
	if trimmedID == "" {
		return nil, errors.New("review id is required")
	}
	if !strings.HasPrefix(trimmedID, "PRR_") {
		return nil, fmt.Errorf("invalid review id %q: must be a GraphQL node id", input.ReviewID)
	}

	trimmedPath := strings.TrimSpace(input.Path)
	if trimmedPath == "" {
		return nil, errors.New("path is required")
	}
	if input.Line <= 0 {
		return nil, errors.New("line must be positive")
	}

	trimmedBody := strings.TrimSpace(input.Body)
	if trimmedBody == "" {
		return nil, errors.New("body is required")
	}

	const mutation = `mutation($input:AddPullRequestReviewThreadInput!){
  addPullRequestReviewThread(input:$input){
    thread { id path isOutdated line }
  }
}`

	graphqlInput := map[string]interface{}{
		"pullRequestReviewId": trimmedID,
		"path":                trimmedPath,
		"line":                input.Line,
		"side":                input.Side,
		"body":                trimmedBody,
	}
	if input.StartLine != nil {
		graphqlInput["startLine"] = *input.StartLine
	}
	if input.StartSide != nil {
		graphqlInput["startSide"] = *input.StartSide
	}

	payload := map[string]interface{}{
		"input": graphqlInput,
	}

	var resp struct {
		AddPullRequestReviewThread struct {
			Thread struct {
				ID         string `json:"id"`
				Path       string `json:"path"`
				IsOutdated bool   `json:"isOutdated"`
				Line       *int   `json:"line"`
			} `json:"thread"`
		} `json:"addPullRequestReviewThread"`
	}

	if err := s.API.GraphQL(mutation, payload, &resp); err != nil {
		return nil, err
	}

	thread := resp.AddPullRequestReviewThread.Thread
	trimmedThreadID := strings.TrimSpace(thread.ID)
	trimmedThreadPath := strings.TrimSpace(thread.Path)
	if trimmedThreadID == "" || trimmedThreadPath == "" {
		return nil, errors.New("addPullRequestReviewThread returned incomplete thread data")
	}

	result := ReviewThread{ID: trimmedThreadID, Path: trimmedThreadPath, IsOutdated: thread.IsOutdated}
	if thread.Line != nil {
		result.Line = thread.Line
	}
	return &result, nil
}

// Submit finalizes a pending review with the given event and optional body.
func (s *Service) Submit(pr resolver.Identity, input SubmitInput) (*SubmitStatus, error) {
	reviewID := strings.TrimSpace(input.ReviewID)
	if reviewID == "" {
		return nil, errors.New("review id is required")
	}

	const query = `mutation SubmitPullRequestReview($input: SubmitPullRequestReviewInput!) {
  submitPullRequestReview(input: $input) {
    pullRequestReview { id state submittedAt databaseId url }
  }
}`

	graphqlInput := map[string]interface{}{
		"pullRequestReviewId": reviewID,
		"event":               input.Event,
	}
	if trimmed := strings.TrimSpace(input.Body); trimmed != "" {
		graphqlInput["body"] = trimmed
	}

	variables := map[string]interface{}{"input": graphqlInput}

	var response struct{}
	if err := s.API.GraphQL(query, variables, &response); err != nil {
		var gqlErr *ghcli.GraphQLError
		if errors.As(err, &gqlErr) {
			return &SubmitStatus{Success: false, Errors: gqlErr.Errors}, nil
		}
		return nil, err
	}

	return &SubmitStatus{Success: true}, nil
}

func (s *Service) currentViewer() (string, error) {
	const query = `query ViewerLogin { viewer { login } }`

	var response struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
	}

	if err := s.API.GraphQL(query, nil, &response); err != nil {
		return "", err
	}

	login := strings.TrimSpace(response.Viewer.Login)
	if login == "" {
		return "", ErrViewerLoginUnavailable
	}

	return login, nil
}

func (s *Service) pullRequestIdentifiers(pr resolver.Identity) (string, string, error) {
	const query = `query($owner:String!,$name:String!,$number:Int!){
  repository(owner:$owner,name:$name){
    pullRequest(number:$number){ id headRefOid }
  }
}`

	variables := map[string]interface{}{
		"owner":  pr.Owner,
		"name":   pr.Repo,
		"number": pr.Number,
	}

	var resp struct {
		Repository struct {
			PullRequest struct {
				ID         string `json:"id"`
				HeadRefOID string `json:"headRefOid"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}

	if err := s.API.GraphQL(query, variables, &resp); err != nil {
		return "", "", err
	}

	nodeID := strings.TrimSpace(resp.Repository.PullRequest.ID)
	headSHA := strings.TrimSpace(resp.Repository.PullRequest.HeadRefOID)
	if nodeID == "" || headSHA == "" {
		return "", "", errors.New("pull request metadata incomplete")
	}

	return nodeID, headSHA, nil
}
