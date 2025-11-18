package beatport

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchClientID(t *testing.T) {
	// Mock the JS file containing the client ID
	jsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `... API_CLIENT_ID: 'test-client-id' ...`)
	}))
	defer jsServer.Close()

	// Mock the docs page that links to the JS file
	docsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The client expects a relative path in the src attribute if it's on the same host,
		// or we can provide a full URL. The refactored code handles relative paths by appending to BaseURL.
		// Let's simulate the structure: BaseURL/docs/ -> HTML -> src="/static/..." -> JS
		// But our mock servers are on different ports.
		// To make this work easily with the current implementation, we can point the src to the jsServer URL.
		// However, the regex expects `src="(/static/btprt/.*\.js)"`.
		// So we must serve the JS from the same server or match the regex.

		// Let's use a single server for both if possible, or just route based on path.
		if r.URL.Path == "/docs/" {
			fmt.Fprintf(w, `<html><script src="/static/btprt/main.js"></script></html>`)
		} else if r.URL.Path == "/static/btprt/main.js" {
			fmt.Fprint(w, `... API_CLIENT_ID: 'test-client-id' ...`)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer docsServer.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	client.BaseURL = docsServer.URL

	err = client.FetchClientID()
	if err != nil {
		t.Fatalf("FetchClientID failed: %v", err)
	}

	if client.ClientID != "test-client-id" {
		t.Errorf("Expected ClientID 'test-client-id', got '%s'", client.ClientID)
	}
}

func TestLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login/" {
			t.Errorf("Expected path /login/, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		var data map[string]string
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
			return
		}
		if data["username"] != "user" || data["password"] != "pass" {
			t.Errorf("Invalid credentials: %v", data)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"username": "user", "email": "user@example.com"}`)
	}))
	defer server.Close()

	client, _ := NewClient()
	client.AuthURL = server.URL

	err := client.Login("user", "pass")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
}

func TestGetGenres(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/catalog/genres/" {
			t.Errorf("Expected path /catalog/genres/, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"results": [{"id": 1, "name": "Techno", "slug": "techno"}]}`)
	}))
	defer server.Close()

	client, _ := NewClient()
	client.BaseURL = server.URL
	client.Token = &OAuthToken{AccessToken: "test-token"}

	genres, err := client.GetGenres()
	if err != nil {
		t.Fatalf("GetGenres failed: %v", err)
	}

	if len(genres) != 1 || genres[0].Name != "Techno" {
		t.Errorf("Unexpected genres: %v", genres)
	}
}

func TestGetTop100(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/catalog/genres/1/top/100" {
			t.Errorf("Expected path /catalog/genres/1/top/100, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("per_page") != "100" {
			t.Errorf("Expected per_page=100")
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"results": [{"id": 101, "name": "Track 1", "artists": [{"name": "Artist 1"}], "mix_name": "Original Mix"}]}`)
	}))
	defer server.Close()

	client, _ := NewClient()
	client.BaseURL = server.URL
	client.Token = &OAuthToken{AccessToken: "test-token"}

	tracks, err := client.GetTop100(1)
	if err != nil {
		t.Fatalf("GetTop100 failed: %v", err)
	}

	if len(tracks) != 1 || tracks[0].Name != "Track 1" {
		t.Errorf("Unexpected tracks: %v", tracks)
	}
}

func TestGetTop100Fallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/catalog/genres/1/top/100" {
			http.NotFound(w, r)
			return
		}

		if r.URL.Path == "/catalog/search" {
			if r.URL.Query().Get("q") != "genre_id:1" {
				t.Errorf("Expected query q=genre_id:1, got %s", r.URL.Query().Get("q"))
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"tracks": [{"id": 101, "name": "Track 1", "artists": [{"name": "Artist 1"}], "mix_name": "Original Mix"}]}`)
			return
		}

		t.Errorf("Unexpected request: %s", r.URL.Path)
	}))
	defer server.Close()

	client, _ := NewClient()
	client.BaseURL = server.URL
	client.Token = &OAuthToken{AccessToken: "test-token"}

	tracks, err := client.GetTop100(1)
	if err != nil {
		t.Fatalf("GetTop100 (fallback) failed: %v", err)
	}

	if len(tracks) != 1 || tracks[0].Name != "Track 1" {
		t.Errorf("Unexpected tracks: %v", tracks)
	}
}
