package pagerduty

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type OnCallGetter interface {
	GetCurrentOnCallEmail(ctx context.Context, scheduleID string) (string, error)
}

type Client struct {
	HTTPClient *http.Client
	APIToken   string
	BaseURL    string
}

type oncallsResponse struct {
	Oncalls []oncallEntry `json:"oncalls"`
}

type oncallEntry struct {
	EscalationLevel int      `json:"escalation_level"`
	User            userRef  `json:"user"`
}

type userRef struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

type userResponse struct {
	User userDetail `json:"user"`
}

type userDetail struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func (c *Client) GetCurrentOnCallEmail(ctx context.Context, scheduleID string) (string, error) {
	now := time.Now().UTC()

	params := url.Values{}
	params.Set("schedule_ids[]", scheduleID)
	params.Set("earliest", "true")
	params.Set("since", now.Format(time.RFC3339))
	params.Set("until", now.Add(1*time.Minute).Format(time.RFC3339))

	reqURL := fmt.Sprintf("%s/oncalls?%s", c.BaseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating oncalls request: %w", err)
	}
	req.Header.Set("Authorization", "Token token="+c.APIToken)
	req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling oncalls API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oncalls API returned status %d", resp.StatusCode)
	}

	var oncalls oncallsResponse
	if err := json.NewDecoder(resp.Body).Decode(&oncalls); err != nil {
		return "", fmt.Errorf("decoding oncalls response: %w", err)
	}

	var userID string
	for _, entry := range oncalls.Oncalls {
		if entry.EscalationLevel == 1 {
			userID = entry.User.ID
			break
		}
	}
	if userID == "" {
		return "", fmt.Errorf("no primary on-call user found for schedule %s", scheduleID)
	}

	// Fetch the user's email
	userURL := fmt.Sprintf("%s/users/%s", c.BaseURL, userID)
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, userURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating user request: %w", err)
	}
	req.Header.Set("Authorization", "Token token="+c.APIToken)
	req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")

	resp, err = c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling users API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("users API returned status %d", resp.StatusCode)
	}

	var userResp userResponse
	if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
		return "", fmt.Errorf("decoding user response: %w", err)
	}

	if userResp.User.Email == "" {
		return "", fmt.Errorf("user %s has no email address", userID)
	}

	return userResp.User.Email, nil
}
