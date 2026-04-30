package ghauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// generateTestKey creates an RSA key pair and returns the private key PEM.
func generateTestKey(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return key, pemBytes
}

func TestGetInstallationToken(t *testing.T) {
	appID := int64(12345)
	installationID := int64(67890)
	expectedToken := "ghs_test_token_abc123"
	key, pemBytes := generateTestKey(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request path.
		wantPath := fmt.Sprintf("/app/installations/%d/access_tokens", installationID)
		if r.URL.Path != wantPath {
			t.Errorf("path = %s, want %s", r.URL.Path, wantPath)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}

		// Validate the JWT.
		auth := r.Header.Get("Authorization")
		if len(auth) <= len("Bearer ") {
			t.Fatal("missing Authorization header")
		}
		tokenStr := auth[len("Bearer "):]

		parsed, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return &key.PublicKey, nil
		})
		if err != nil {
			t.Fatalf("parsing JWT: %v", err)
		}

		claims, ok := parsed.Claims.(jwt.MapClaims)
		if !ok {
			t.Fatal("unexpected claims type")
		}

		iss, _ := claims["iss"].(string)
		if iss != strconv.FormatInt(appID, 10) {
			t.Errorf("iss = %s, want %d", iss, appID)
		}

		exp, _ := claims["exp"].(float64)
		expTime := time.Unix(int64(exp), 0)
		if time.Until(expTime) < 9*time.Minute || time.Until(expTime) > 11*time.Minute {
			t.Errorf("exp not ~10 minutes from now: %v", expTime)
		}

		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"token": "%s", "expires_at": "2099-01-01T00:00:00Z"}`, expectedToken)
	}))
	defer server.Close()

	token, _, err := GetInstallationToken(context.Background(), server.Client(), server.URL, appID, installationID, pemBytes)
	if err != nil {
		t.Fatalf("GetInstallationToken() error: %v", err)
	}
	if token != expectedToken {
		t.Errorf("token = %s, want %s", token, expectedToken)
	}
}

func TestGetInstallationTokenInvalidPEM(t *testing.T) {
	_, _, err := GetInstallationToken(context.Background(), http.DefaultClient, "https://unused", 1, 1, []byte("not a key"))
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}

func TestGetInstallationTokenAPIError(t *testing.T) {
	_, pemBytes := generateTestKey(t)

	tests := []struct {
		name       string
		status     int
		body       string
		wantSubstr string
	}{
		{
			name:       "unauthorized",
			status:     http.StatusUnauthorized,
			body:       `{"message": "Bad credentials"}`,
			wantSubstr: "status 401",
		},
		{
			name:       "not found",
			status:     http.StatusNotFound,
			body:       `{"message": "Not Found"}`,
			wantSubstr: "status 404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()

			_, _, err := GetInstallationToken(context.Background(), server.Client(), server.URL, 1, 1, pemBytes)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := err.Error(); !contains(got, tt.wantSubstr) {
				t.Errorf("error = %q, want substring %q", got, tt.wantSubstr)
			}
		})
	}
}

func TestGetInstallationTokenEmptyToken(t *testing.T) {
	_, pemBytes := generateTestKey(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"token": ""}`)
	}))
	defer server.Close()

	_, _, err := GetInstallationToken(context.Background(), server.Client(), server.URL, 1, 1, pemBytes)
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
