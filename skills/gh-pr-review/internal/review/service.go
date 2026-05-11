package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/agynio/gh-pr-review/internal/ghcli"
	"github.com/agynio/gh-pr-review/internal/resolver"
)

// Service coordinates review GraphQL operations through the gh CLI.
type Service struct {
	API ghcli.API
}

const similarityThreshold = 0.8

func calculateSimilarity(s1, s2 string) float64 {
	w1 := tokenize(s1)
	w2 := tokenize(s2)
	if len(w1) == 0 || len(w2) == 0 {
		return 0
	}

	m1 := make(map[string]struct{})
	for _, w := range w1 {
		m1[w] = struct{}{}
	}

	m2 := make(map[string]struct{})
	for _, w := range w2 {
		m2[w] = struct{}{}
	}

	intersect := 0
	for w := range m1 {
		if _, ok := m2[w]; ok {
			intersect++
		}
	}

	union := len(m1)
	for w := range m2 {
		if _, ok := m1[w]; !ok {
			union++
		}
	}

	return float64(intersect) / float64(union)
}

func tokenize(s string) []string {
	s = strings.ToLower(s)
	f := func(c rune) bool {
		return !unicode.IsLetter(c) && !unicode.IsNumber(c)
	}
	return strings.FieldsFunc(s, f)
}

func (s *Service) fetchExistingCommentBodies(pr resolver.Identity) ([]string, error) {
	const query = `query($owner:String!,$name:String!,$number:Int!){
  repository(owner:$owner,name:$name){
    pullRequest(number:$number){
      reviewThreads(first:100){
        nodes {
          comments(first:100){
            nodes {
              body
            }
          }
        }
      }
    }
  }
}`

	variables := map[string]interface{}{
		"owner":  pr.Owner,
		"name":   pr.Repo,
		"number": pr.Number,
	}

	var resp struct {
		Repository *struct {
			PullRequest *struct {
				ReviewThreads struct {
					Nodes []struct {
						Comments struct {
							Nodes []struct {
								Body string `json:"body"`
							} `json:"nodes"`
						} `json:"comments"`
					} `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}

	if err := s.API.GraphQL(query, variables, &resp); err != nil {
		return nil, fmt.Errorf("fetching existing comments: %w", err)
	}

	if resp.Repository == nil || resp.Repository.PullRequest == nil {
		return nil, nil
	}

	var bodies []string
	for _, t := range resp.Repository.PullRequest.ReviewThreads.Nodes {
		for _, c := range t.Comments.Nodes {
			bodies = append(bodies, c.Body)
		}
	}
	return bodies, nil
}

func (s *Service) checkForDuplicates(pr resolver.Identity, body string) error {
	existing, err := s.fetchExistingCommentBodies(pr)
	if err != nil {
		return err
	}

	trimmedBody := strings.TrimSpace(body)
	for _, b := range existing {
		if calculateSimilarity(trimmedBody, b) >= similarityThreshold {
			return fmt.Errorf("a similar comment already exists in this pull request; skipping duplicate comment (similarity: %.2f)", calculateSimilarity(trimmedBody, b))
		}
	}

	return nil
}

// AddThread adds an inline review comment thread to an existing pending review.
func (s *Service) AddThread(pr resolver.Identity, input ThreadInput) (*ReviewThread, error) {
	if err := s.checkForDuplicates(pr, input.Body); err != nil {
		return nil, err
	}

	return s.addThreadNoCheck(pr, input)
}

func (s *Service) addThreadNoCheck(pr resolver.Identity, input ThreadInput) (*ReviewThread, error) {
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

// AddBatch adds multiple inline review comment threads, filtering out duplicates by similarity.
func (s *Service) AddBatch(pr resolver.Identity, inputs []ThreadInput) ([]ReviewThread, error) {
	existing, err := s.fetchExistingCommentBodies(pr)
	if err != nil {
		return nil, err
	}

	var results []ReviewThread
	var acceptedBodies []string

	for _, input := range inputs {
		isDuplicate := false
		trimmedBody := strings.TrimSpace(input.Body)

		// Check against existing comments
		for _, b := range existing {
			if calculateSimilarity(trimmedBody, b) >= similarityThreshold {
				isDuplicate = true
				break
			}
		}
		if isDuplicate {
			continue
		}

		// Check against already accepted comments in this batch
		for _, b := range acceptedBodies {
			if calculateSimilarity(trimmedBody, b) >= similarityThreshold {
				isDuplicate = true
				break
			}
		}
		if isDuplicate {
			continue
		}

		thread, err := s.addThreadNoCheck(pr, input)
		if err != nil {
			return nil, err
		}
		results = append(results, *thread)
		acceptedBodies = append(acceptedBodies, trimmedBody)
	}

	return results, nil
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
