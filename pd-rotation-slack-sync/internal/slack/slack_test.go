package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLookupUserByEmail_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users.lookupByEmail" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("email") != "jane@example.com" {
			t.Errorf("unexpected email param: %s", r.URL.Query().Get("email"))
		}
		if r.Header.Get("Authorization") != "Bearer xoxb-test" {
			t.Errorf("unexpected auth: %s", r.Header.Get("Authorization"))
		}
		fmt.Fprint(w, `{"ok": true, "user": {"id": "U123"}}`)
	}))
	defer srv.Close()

	client := &Client{HTTPClient: srv.Client(), APIToken: "xoxb-test", BaseURL: srv.URL}
	id, err := client.LookupUserByEmail(context.Background(), "jane@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "U123" {
		t.Errorf("id = %q, want %q", id, "U123")
	}
}

func TestLookupUserByEmail_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok": false, "error": "users_not_found"}`)
	}))
	defer srv.Close()

	client := &Client{HTTPClient: srv.Client(), APIToken: "xoxb-test", BaseURL: srv.URL}
	_, err := client.LookupUserByEmail(context.Background(), "nobody@example.com")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpdateUserGroupMembers_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/usergroups.users.update" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}

		var body updateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		if body.UserGroup != "S456" {
			t.Errorf("usergroup = %q, want %q", body.UserGroup, "S456")
		}
		if body.Users != "U123" {
			t.Errorf("users = %q, want %q", body.Users, "U123")
		}

		fmt.Fprint(w, `{"ok": true}`)
	}))
	defer srv.Close()

	client := &Client{HTTPClient: srv.Client(), APIToken: "xoxb-test", BaseURL: srv.URL}
	err := client.UpdateUserGroupMembers(context.Background(), "S456", []string{"U123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateUserGroupMembers_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok": false, "error": "invalid_auth"}`)
	}))
	defer srv.Close()

	client := &Client{HTTPClient: srv.Client(), APIToken: "xoxb-test", BaseURL: srv.URL}
	err := client.UpdateUserGroupMembers(context.Background(), "S456", []string{"U123"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
