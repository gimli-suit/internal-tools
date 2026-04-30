package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// ProjectQuerier fetches project items from a GitHub Project V2.
type ProjectQuerier interface {
	GetProjectItems(ctx context.Context) (*ProjectData, error)
}

// AncestorChecker checks if a commit is an ancestor of another.
type AncestorChecker interface {
	IsAncestor(ctx context.Context, owner, repo, commitSHA, deployedSHA string) (bool, error)
}

// StatusUpdater updates a project item's status field.
type StatusUpdater interface {
	UpdateItemStatus(ctx context.Context, projectID, itemID, fieldID, optionID string) error
}

// IterationUpdater updates a project item's iteration field.
type IterationUpdater interface {
	UpdateItemIteration(ctx context.Context, projectID, itemID, fieldID, iterationID string) error
}

// Iteration represents a project iteration (sprint).
type Iteration struct {
	ID        string
	Title     string
	StartDate string // "2026-01-01" format
	Duration  int    // days
}

// ProjectData holds the project metadata and all items.
type ProjectData struct {
	ProjectID        string
	StatusFieldID    string
	ShippedOptionID  string
	StatusOptions    []string
	IterationFieldID string
	Iterations       []Iteration
	Items            []ProjectItem
}

// ProjectItem is a single item in the project board.
type ProjectItem struct {
	ItemID             string
	CurrentStatus      string
	CurrentIterationID string
	Issue              *Issue
}

// Issue is a GitHub issue with its closing PRs.
type Issue struct {
	Number     int
	Title      string
	Closed     bool
	ClosedAt   string // ISO 8601 timestamp, e.g. "2026-04-15T10:30:00Z"
	Repository string // "owner/repo"
	ClosingPRs []PullRequest
}

// PullRequest is a PR linked to an issue.
type PullRequest struct {
	Number      int
	Merged      bool
	MergeCommit string
	Repository  string // "owner/repo"
}

// Client implements ProjectQuerier, AncestorChecker, and StatusUpdater.
type Client struct {
	HTTPClient    *http.Client
	Token         string
	GraphQLURL    string
	RestBaseURL   string
	Org           string
	ProjectNumber int

	ancestorCache map[string]bool // cache for IsAncestor results, keyed by commitSHA
}

func (c *Client) logWarning(msg string) {
	slog.Warn(msg)
}

// graphqlRequest sends a GraphQL query and decodes the response.
// If the response contains both data and errors (partial success),
// the data is decoded and errors are returned separately as warnings.
func (c *Client) graphqlRequest(ctx context.Context, query string, variables map[string]any, result any) error {
	body := map[string]any{
		"query":     query,
		"variables": variables,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling graphql request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.GraphQLURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating graphql request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing graphql request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading graphql response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("graphql returned status %d: %s", resp.StatusCode, respBody)
	}

	var gqlResp struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return fmt.Errorf("decoding graphql response: %w", err)
	}

	hasData := len(gqlResp.Data) > 0 && string(gqlResp.Data) != "null"

	// If there are errors but no data, fail hard.
	if len(gqlResp.Errors) > 0 && !hasData {
		return fmt.Errorf("graphql error: %s", gqlResp.Errors[0].Message)
	}

	// If there is data, decode it. Errors alongside data are partial
	// failures (e.g. inaccessible issues in a project) — log and continue.
	if result != nil && hasData {
		if err := json.Unmarshal(gqlResp.Data, result); err != nil {
			return fmt.Errorf("decoding graphql data: %w", err)
		}
	}

	if len(gqlResp.Errors) > 0 {
		for _, e := range gqlResp.Errors {
			c.logWarning("graphql partial error (some items may be inaccessible): " + e.Message)
		}
	}

	return nil
}

// GraphQL query for fetching project items with their linked PRs.
const projectItemsQuery = `
query($org: String!, $number: Int!, $cursor: String) {
  organization(login: $org) {
    projectV2(number: $number) {
      id
      statusField: field(name: "Status") {
        ... on ProjectV2SingleSelectField {
          id
          options {
            id
            name
          }
        }
      }
      iterationField: field(name: "Iteration") {
        ... on ProjectV2IterationField {
          id
          configuration {
            iterations {
              id
              title
              startDate
              duration
            }
            completedIterations {
              id
              title
              startDate
              duration
            }
          }
        }
      }
      items(first: 100, after: $cursor) {
        totalCount
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          id
          statusValue: fieldValueByName(name: "Status") {
            ... on ProjectV2ItemFieldSingleSelectValue {
              name
            }
          }
          iterationValue: fieldValueByName(name: "Iteration") {
            ... on ProjectV2ItemFieldIterationValue {
              iterationId
            }
          }
          content {
            ... on Issue {
              number
              title
              state
              closedAt
              repository {
                nameWithOwner
              }
              timelineItems(itemTypes: [CROSS_REFERENCED_EVENT, CONNECTED_EVENT], first: 50) {
                nodes {
                  __typename
                  ... on CrossReferencedEvent {
                    willCloseTarget
                    source {
                      ... on PullRequest {
                        number
                        merged
                        mergeCommit {
                          oid
                        }
                        repository {
                          nameWithOwner
                        }
                      }
                    }
                  }
                  ... on ConnectedEvent {
                    subject {
                      ... on PullRequest {
                        number
                        merged
                        mergeCommit {
                          oid
                        }
                        repository {
                          nameWithOwner
                        }
                      }
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}
`

// Response types for the project items query.
type projectQueryResponse struct {
	Organization struct {
		ProjectV2 struct {
			ID          string `json:"id"`
			StatusField struct {
				ID      string `json:"id"`
				Options []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"options"`
			} `json:"statusField"`
			IterationField struct {
				ID            string `json:"id"`
				Configuration struct {
					Iterations []struct {
						ID        string `json:"id"`
						Title     string `json:"title"`
						StartDate string `json:"startDate"`
						Duration  int    `json:"duration"`
					} `json:"iterations"`
					CompletedIterations []struct {
						ID        string `json:"id"`
						Title     string `json:"title"`
						StartDate string `json:"startDate"`
						Duration  int    `json:"duration"`
					} `json:"completedIterations"`
				} `json:"configuration"`
			} `json:"iterationField"`
			Items struct {
				TotalCount int `json:"totalCount"`
				PageInfo   struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []projectItemNode `json:"nodes"`
			} `json:"items"`
		} `json:"projectV2"`
	} `json:"organization"`
}

type projectItemNode struct {
	ID          string `json:"id"`
	StatusValue struct {
		Name string `json:"name"`
	} `json:"statusValue"`
	IterationValue struct {
		IterationID string `json:"iterationId"`
	} `json:"iterationValue"`
	Content struct {
		Number   int    `json:"number"`
		Title    string `json:"title"`
		State    string `json:"state"`
		ClosedAt string `json:"closedAt"`
		Repository *struct {
			NameWithOwner string `json:"nameWithOwner"`
		} `json:"repository"`
		TimelineItems *struct {
			Nodes []timelineEventNode `json:"nodes"`
		} `json:"timelineItems"`
	} `json:"content"`
}

type timelineEventNode struct {
	Typename string `json:"__typename"`
	// CrossReferencedEvent fields
	WillCloseTarget bool `json:"willCloseTarget"`
	Source          struct {
		Number      int  `json:"number"`
		Merged      bool `json:"merged"`
		MergeCommit *struct {
			OID string `json:"oid"`
		} `json:"mergeCommit"`
		Repository *struct {
			NameWithOwner string `json:"nameWithOwner"`
		} `json:"repository"`
	} `json:"source"`
	// ConnectedEvent fields
	Subject struct {
		Number      int  `json:"number"`
		Merged      bool `json:"merged"`
		MergeCommit *struct {
			OID string `json:"oid"`
		} `json:"mergeCommit"`
		Repository *struct {
			NameWithOwner string `json:"nameWithOwner"`
		} `json:"repository"`
	} `json:"subject"`
}

// GetProjectItems fetches all items from the configured project.
func (c *Client) GetProjectItems(ctx context.Context) (*ProjectData, error) {
	pd := &ProjectData{}
	var cursor *string

	for {
		vars := map[string]any{
			"org":    c.Org,
			"number": c.ProjectNumber,
		}
		if cursor != nil {
			vars["cursor"] = *cursor
		}

		var resp projectQueryResponse
		if err := c.graphqlRequest(ctx, projectItemsQuery, vars, &resp); err != nil {
			return nil, fmt.Errorf("querying project items: %w", err)
		}

		proj := resp.Organization.ProjectV2

			if pd.ProjectID == "" {
				slog.Info("project total items", "total", proj.Items.TotalCount)
			}

		// Set project metadata on first page.
		if pd.ProjectID == "" {
			pd.ProjectID = proj.ID
			pd.StatusFieldID = proj.StatusField.ID
			for _, opt := range proj.StatusField.Options {
				pd.StatusOptions = append(pd.StatusOptions, opt.Name)
				normalized := strings.Join(strings.Fields(opt.Name), " ")
				if normalized == "🚢 Shipped" {
					pd.ShippedOptionID = opt.ID
				}
			}
			pd.IterationFieldID = proj.IterationField.ID
			for _, it := range proj.IterationField.Configuration.Iterations {
				pd.Iterations = append(pd.Iterations, Iteration{ID: it.ID, Title: it.Title, StartDate: it.StartDate, Duration: it.Duration})
			}
			for _, it := range proj.IterationField.Configuration.CompletedIterations {
				pd.Iterations = append(pd.Iterations, Iteration{ID: it.ID, Title: it.Title, StartDate: it.StartDate, Duration: it.Duration})
			}
		}

		for _, node := range proj.Items.Nodes {
			item := ProjectItem{
				ItemID:             node.ID,
				CurrentStatus:      node.StatusValue.Name,
				CurrentIterationID: node.IterationValue.IterationID,
			}

			// Only process Issue content (skip DraftIssues, PRs added directly).
			if node.Content.Repository != nil && node.Content.Number > 0 {
				issue := &Issue{
					Number:     node.Content.Number,
					Title:      node.Content.Title,
					Closed:     node.Content.State == "CLOSED",
					ClosedAt:   node.Content.ClosedAt,
					Repository: node.Content.Repository.NameWithOwner,
				}
				if timeline := node.Content.TimelineItems; timeline != nil {
					seen := make(map[int]bool)
					for _, event := range timeline.Nodes {
						var prNum int
						var prMerged bool
						var prCommit *struct{ OID string `json:"oid"` }
						var prRepo *struct{ NameWithOwner string `json:"nameWithOwner"` }

						switch event.Typename {
						case "CrossReferencedEvent":
							// Include merged cross-referenced PRs even without
							// willCloseTarget, since many teams link PRs to issues
							// without using "Fixes/Closes" keywords.
							if !event.Source.Merged {
								continue
							}
							prNum = event.Source.Number
							prMerged = event.Source.Merged
							prCommit = event.Source.MergeCommit
							prRepo = event.Source.Repository
						case "ConnectedEvent":
							prNum = event.Subject.Number
							prMerged = event.Subject.Merged
							prCommit = event.Subject.MergeCommit
							prRepo = event.Subject.Repository
						default:
							continue
						}

						if prRepo == nil || prNum == 0 {
							continue
						}
						if seen[prNum] {
							continue
						}
						seen[prNum] = true

						p := PullRequest{
							Number:     prNum,
							Merged:     prMerged,
							Repository: prRepo.NameWithOwner,
						}
						if prCommit != nil {
							p.MergeCommit = prCommit.OID
						}
						issue.ClosingPRs = append(issue.ClosingPRs, p)
					}
				}
				item.Issue = issue
			}

			pd.Items = append(pd.Items, item)
		}

		if !proj.Items.PageInfo.HasNextPage {
			break
		}
		cursor = &proj.Items.PageInfo.EndCursor
	}

	return pd, nil
}

// IsAncestor checks if commitSHA is included in the history up to deployedSHA
// using the GitHub compare API. The comparison is done as deployedSHA...commitSHA
// which works correctly with squash-merged PRs. Results are cached by commitSHA.
func (c *Client) IsAncestor(ctx context.Context, owner, repo, commitSHA, deployedSHA string) (bool, error) {
	if c.ancestorCache == nil {
		c.ancestorCache = make(map[string]bool)
	}
	if result, ok := c.ancestorCache[commitSHA]; ok {
		return result, nil
	}

	url := fmt.Sprintf("%s/repos/%s/%s/compare/%s...%s", c.RestBaseURL, owner, repo, deployedSHA, commitSHA)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("creating compare request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("executing compare request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("compare API returned status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decoding compare response: %w", err)
	}

	// With deployedSHA...commitSHA:
	// "behind" = commitSHA is behind deployedSHA (deployed)
	// "identical" = same commit
	isAnc := result.Status == "behind" || result.Status == "identical"
	c.ancestorCache[commitSHA] = isAnc
	return isAnc, nil
}

// GraphQL mutation for updating a project item's field value.
const updateStatusMutation = `
mutation($projectId: ID!, $itemId: ID!, $fieldId: ID!, $optionId: String!) {
  updateProjectV2ItemFieldValue(
    input: {
      projectId: $projectId
      itemId: $itemId
      fieldId: $fieldId
      value: { singleSelectOptionId: $optionId }
    }
  ) {
    projectV2Item {
      id
    }
  }
}
`

// UpdateItemStatus sets a project item's Status field to the given option.
func (c *Client) UpdateItemStatus(ctx context.Context, projectID, itemID, fieldID, optionID string) error {
	vars := map[string]any{
		"projectId": projectID,
		"itemId":    itemID,
		"fieldId":   fieldID,
		"optionId":  optionID,
	}
	return c.graphqlRequest(ctx, updateStatusMutation, vars, nil)
}

// GraphQL mutation for updating a project item's iteration field.
const updateIterationMutation = `
mutation($projectId: ID!, $itemId: ID!, $fieldId: ID!, $iterationId: String!) {
  updateProjectV2ItemFieldValue(
    input: {
      projectId: $projectId
      itemId: $itemId
      fieldId: $fieldId
      value: { iterationId: $iterationId }
    }
  ) {
    projectV2Item {
      id
    }
  }
}
`

// UpdateItemIteration sets a project item's Iteration field.
func (c *Client) UpdateItemIteration(ctx context.Context, projectID, itemID, fieldID, iterationID string) error {
	vars := map[string]any{
		"projectId":   projectID,
		"itemId":      itemID,
		"fieldId":     fieldID,
		"iterationId": iterationID,
	}
	return c.graphqlRequest(ctx, updateIterationMutation, vars, nil)
}
