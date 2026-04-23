package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// ProjectData holds the project metadata and all items.
type ProjectData struct {
	ProjectID       string
	StatusFieldID   string
	ShippedOptionID string
	Items           []ProjectItem
}

// ProjectItem is a single item in the project board.
type ProjectItem struct {
	ItemID        string
	CurrentStatus string
	Issue         *Issue
}

// Issue is a GitHub issue with its closing PRs.
type Issue struct {
	Number     int
	Title      string
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
}

// graphqlRequest sends a GraphQL query and decodes the response.
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
	if len(gqlResp.Errors) > 0 {
		return fmt.Errorf("graphql error: %s", gqlResp.Errors[0].Message)
	}

	if result != nil {
		if err := json.Unmarshal(gqlResp.Data, result); err != nil {
			return fmt.Errorf("decoding graphql data: %w", err)
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
      field(name: "Status") {
        ... on ProjectV2SingleSelectField {
          id
          options {
            id
            name
          }
        }
      }
      items(first: 100, after: $cursor) {
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          id
          fieldValueByName(name: "Status") {
            ... on ProjectV2ItemFieldSingleSelectValue {
              name
            }
          }
          content {
            ... on Issue {
              number
              title
              repository {
                nameWithOwner
              }
              closingPullRequestsReferences(first: 50) {
                nodes {
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
`

// Response types for the project items query.
type projectQueryResponse struct {
	Organization struct {
		ProjectV2 struct {
			ID    string `json:"id"`
			Field struct {
				ID      string `json:"id"`
				Options []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"options"`
			} `json:"field"`
			Items struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []projectItemNode `json:"nodes"`
			} `json:"items"`
		} `json:"projectV2"`
	} `json:"organization"`
}

type projectItemNode struct {
	ID               string `json:"id"`
	FieldValueByName struct {
		Name string `json:"name"`
	} `json:"fieldValueByName"`
	Content struct {
		Number     int    `json:"number"`
		Title      string `json:"title"`
		Repository *struct {
			NameWithOwner string `json:"nameWithOwner"`
		} `json:"repository"`
		ClosingPullRequestsReferences *struct {
			Nodes []prNode `json:"nodes"`
		} `json:"closingPullRequestsReferences"`
	} `json:"content"`
}

type prNode struct {
	Number      int  `json:"number"`
	Merged      bool `json:"merged"`
	MergeCommit *struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
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

		// Set project metadata on first page.
		if pd.ProjectID == "" {
			pd.ProjectID = proj.ID
			pd.StatusFieldID = proj.Field.ID
			for _, opt := range proj.Field.Options {
				if opt.Name == "🚢 Shipped" {
					pd.ShippedOptionID = opt.ID
					break
				}
			}
		}

		for _, node := range proj.Items.Nodes {
			item := ProjectItem{
				ItemID:        node.ID,
				CurrentStatus: node.FieldValueByName.Name,
			}

			// Only process Issue content (skip DraftIssues, PRs added directly).
			if node.Content.Repository != nil && node.Content.Number > 0 {
				issue := &Issue{
					Number:     node.Content.Number,
					Title:      node.Content.Title,
					Repository: node.Content.Repository.NameWithOwner,
				}
				if refs := node.Content.ClosingPullRequestsReferences; refs != nil {
					for _, pr := range refs.Nodes {
						p := PullRequest{
							Number:     pr.Number,
							Merged:     pr.Merged,
							Repository: pr.Repository.NameWithOwner,
						}
						if pr.MergeCommit != nil {
							p.MergeCommit = pr.MergeCommit.OID
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

// IsAncestor checks if commitSHA is an ancestor of deployedSHA using
// the GitHub compare API.
func (c *Client) IsAncestor(ctx context.Context, owner, repo, commitSHA, deployedSHA string) (bool, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/compare/%s...%s", c.RestBaseURL, owner, repo, commitSHA, deployedSHA)

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

	// "behind" = commitSHA is behind deployedSHA (ancestor)
	// "identical" = same commit
	return result.Status == "behind" || result.Status == "identical", nil
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
