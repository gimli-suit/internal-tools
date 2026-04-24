package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type UserGroupUpdater interface {
	LookupUserByEmail(ctx context.Context, email string) (string, error)
	GetUserGroupMembers(ctx context.Context, groupID string) ([]string, error)
	UpdateUserGroupMembers(ctx context.Context, groupID string, userIDs []string) error
	PostMessage(ctx context.Context, channelID, text string) error
}

type Client struct {
	HTTPClient *http.Client
	APIToken   string
	BaseURL    string
}

type slackResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type lookupResponse struct {
	slackResponse
	User struct {
		ID string `json:"id"`
	} `json:"user"`
}

type updateRequest struct {
	UserGroup string `json:"usergroup"`
	Users     string `json:"users"`
}

func (c *Client) LookupUserByEmail(ctx context.Context, email string) (string, error) {
	reqURL := fmt.Sprintf("%s/users.lookupByEmail?email=%s", c.BaseURL, url.QueryEscape(email))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating lookup request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling users.lookupByEmail: %w", err)
	}
	defer resp.Body.Close()

	var result lookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding lookup response: %w", err)
	}

	if !result.OK {
		return "", fmt.Errorf("users.lookupByEmail failed: %s", result.Error)
	}

	return result.User.ID, nil
}

type listUsersResponse struct {
	slackResponse
	Users []string `json:"users"`
}

func (c *Client) GetUserGroupMembers(ctx context.Context, groupID string) ([]string, error) {
	reqURL := fmt.Sprintf("%s/usergroups.users.list?usergroup=%s", c.BaseURL, url.QueryEscape(groupID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating list users request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIToken)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling usergroups.users.list: %w", err)
	}
	defer resp.Body.Close()

	var result listUsersResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding list users response: %w", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("usergroups.users.list failed: %s", result.Error)
	}

	return result.Users, nil
}

func (c *Client) UpdateUserGroupMembers(ctx context.Context, groupID string, userIDs []string) error {
	body := updateRequest{
		UserGroup: groupID,
		Users:     strings.Join(userIDs, ","),
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling update request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/usergroups.users.update", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating update request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling usergroups.users.update: %w", err)
	}
	defer resp.Body.Close()

	var result slackResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding update response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("usergroups.users.update failed: %s", result.Error)
	}

	return nil
}

type postMessageRequest struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

// PostMessage sends a message to a channel or DM. When channelID is a user ID,
// Slack automatically opens/reuses a DM conversation.
func (c *Client) PostMessage(ctx context.Context, channelID, text string) error {
	body := postMessageRequest{
		Channel: channelID,
		Text:    text,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling message request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat.postMessage", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating message request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling chat.postMessage: %w", err)
	}
	defer resp.Body.Close()

	var result slackResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding message response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("chat.postMessage failed: %s", result.Error)
	}

	return nil
}
