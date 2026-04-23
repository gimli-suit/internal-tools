package pagerduty

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetCurrentOnCallEmail_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/oncalls":
			if r.Header.Get("Authorization") != "Token token=test-token" {
				t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
			}
			fmt.Fprint(w, `{
				"oncalls": [{
					"escalation_level": 1,
					"user": {"id": "PUSER1", "summary": "Jane Doe"}
				}]
			}`)
		case r.URL.Path == "/users/PUSER1":
			fmt.Fprint(w, `{"user": {"id": "PUSER1", "email": "jane@example.com"}}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		APIToken:   "test-token",
		BaseURL:    srv.URL,
	}

	email, err := client.GetCurrentOnCallEmail(context.Background(), "PSCHED1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "jane@example.com" {
		t.Errorf("email = %q, want %q", email, "jane@example.com")
	}
}

func TestGetCurrentOnCallEmail_NoOncalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"oncalls": []}`)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		APIToken:   "test-token",
		BaseURL:    srv.URL,
	}

	_, err := client.GetCurrentOnCallEmail(context.Background(), "PSCHED1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetCurrentOnCallEmail_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		APIToken:   "test-token",
		BaseURL:    srv.URL,
	}

	_, err := client.GetCurrentOnCallEmail(context.Background(), "PSCHED1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetCurrentOnCallEmail_NoEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/oncalls":
			fmt.Fprint(w, `{"oncalls": [{"escalation_level": 1, "user": {"id": "PUSER1", "summary": "Jane"}}]}`)
		case r.URL.Path == "/users/PUSER1":
			fmt.Fprint(w, `{"user": {"id": "PUSER1", "email": ""}}`)
		}
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		APIToken:   "test-token",
		BaseURL:    srv.URL,
	}

	_, err := client.GetCurrentOnCallEmail(context.Background(), "PSCHED1")
	if err == nil {
		t.Fatal("expected error for empty email, got nil")
	}
}

func TestGetCurrentOnCallEmail_PicksLevel1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/oncalls":
			fmt.Fprint(w, `{
				"oncalls": [
					{"escalation_level": 2, "user": {"id": "PUSER2", "summary": "Bob"}},
					{"escalation_level": 1, "user": {"id": "PUSER1", "summary": "Jane"}}
				]
			}`)
		case r.URL.Path == "/users/PUSER1":
			fmt.Fprint(w, `{"user": {"id": "PUSER1", "email": "jane@example.com"}}`)
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: srv.Client(),
		APIToken:   "test-token",
		BaseURL:    srv.URL,
	}

	email, err := client.GetCurrentOnCallEmail(context.Background(), "PSCHED1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "jane@example.com" {
		t.Errorf("email = %q, want %q", email, "jane@example.com")
	}
}
