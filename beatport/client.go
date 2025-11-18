package beatport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	DefaultAPIBaseURL  = "https://api.beatport.com/v4"
	DefaultAuthBaseURL = "https://api.beatport.com/v4/auth"
	TokenFile          = "token.json"
	MaxRetries         = 3
)

type Client struct {
	HTTPClient *http.Client
	Token      *OAuthToken
	ClientID   string
	BaseURL    string
	AuthURL    string
}

func NewClient() (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		HTTPClient: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
		},
		BaseURL: DefaultAPIBaseURL,
		AuthURL: DefaultAuthBaseURL,
	}, nil
}

// doRequest performs an HTTP request with exponential backoff retry
func (c *Client) doRequest(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for i := 0; i <= MaxRetries; i++ {
		if i > 0 {
			time.Sleep(time.Duration(1<<uint(i)) * time.Second) // 2s, 4s, 8s
		}
		resp, err = c.HTTPClient.Do(req)
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
	}
	return resp, err
}

func (c *Client) LoadToken() error {
	file, err := os.Open(TokenFile)
	if err != nil {
		return err
	}
	defer file.Close()

	var token OAuthToken
	if err := json.NewDecoder(file).Decode(&token); err != nil {
		return err
	}
	c.Token = &token
	return nil
}

func (c *Client) SaveToken() error {
	if c.Token == nil {
		return fmt.Errorf("no token to save")
	}
	file, err := os.Create(TokenFile)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(c.Token)
}

func (c *Client) FetchClientID() error {
	req, err := http.NewRequest("GET", c.BaseURL+"/docs/", nil)
	if err != nil {
		return err
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Find script src
	reScript := regexp.MustCompile(`src="(/static/btprt/.*\.js)"`)
	matches := reScript.FindAllStringSubmatch(string(body), -1)

	for _, match := range matches {
		// Handle relative URLs correctly if we are mocking
		scriptURL := match[1]
		if !strings.HasPrefix(scriptURL, "http") {
			// If BaseURL is the real one, we might need to be careful,
			// but usually the script src is relative path.
			// In the original code it was hardcoded https://api.beatport.com
			// For testing, we want it to be c.BaseURL (or root of server)
			// The regex captures /static/..., so we can append to a base.
			// However, the original code did: "https://api.beatport.com" + match[1]
			// Let's use a helper or just assume BaseURL root.
			// For the real API, BaseURL is .../v4, but the script is at root /static.
			// So we need the host.

			u, err := url.Parse(c.BaseURL)
			if err == nil {
				scriptURL = fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, match[1])
			} else {
				scriptURL = "https://api.beatport.com" + match[1]
			}
		}

		reqScript, _ := http.NewRequest("GET", scriptURL, nil)
		scriptResp, err := c.doRequest(reqScript)
		if err != nil {
			continue
		}
		defer scriptResp.Body.Close()

		jsBody, err := io.ReadAll(scriptResp.Body)
		if err != nil {
			continue
		}

		// Find client_id
		reClientID := regexp.MustCompile(`API_CLIENT_ID: \'(.*)\'`)
		clientMatches := reClientID.FindAllStringSubmatch(string(jsBody), -1)
		if len(clientMatches) > 0 {
			c.ClientID = clientMatches[0][1]
			return nil
		}
	}

	return fmt.Errorf("could not fetch API_CLIENT_ID")
}

func (c *Client) Login(username, password string) error {
	// Try loading token first
	if err := c.LoadToken(); err == nil {
		// Validate token (optional, but good practice)
		// For now, we assume it's valid or will fail later
		return nil
	}

	loginURL := c.AuthURL + "/login/"
	data := map[string]string{
		"username": username,
		"password": password,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var res map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return err
	}

	if _, ok := res["username"]; !ok {
		return fmt.Errorf("login failed: %v", res)
	}

	return nil
}

func (c *Client) Authorize() (string, error) {
	// If we already have a token, skip authorization
	if c.Token != nil {
		return "", nil
	}

	if c.ClientID == "" {
		if err := c.FetchClientID(); err != nil {
			return "", err
		}
	}

	redirectURI := c.AuthURL + "/o/post-message/"
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", c.ClientID)
	params.Set("redirect_uri", redirectURI)

	authURL := c.AuthURL + "/o/authorize/?" + params.Encode()

	// We need to prevent redirects to capture the Location header
	c.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	defer func() { c.HTTPClient.CheckRedirect = nil }()

	req, err := http.NewRequest("GET", authURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	location := resp.Header.Get("Location")
	if location == "" {
		// Check for error in body
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("authorization failed, no location header. Body: %s", string(body))
	}

	locURL, err := url.Parse(location)
	if err != nil {
		return "", err
	}

	code := locURL.Query().Get("code")
	if code == "" {
		return "", fmt.Errorf("authorization failed, no code in location: %s", location)
	}

	return code, nil
}

func (c *Client) GetToken(code string) error {
	if c.Token != nil {
		return nil
	}

	tokenURL := c.AuthURL + "/o/token/"
	redirectURI := c.AuthURL + "/o/post-message/"

	data := url.Values{}
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", c.ClientID)

	// PostForm uses Client.PostForm which doesn't use our doRequest wrapper easily
	// Let's construct a request
	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get token: %s", string(body))
	}

	var token OAuthToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return err
	}

	c.Token = &token
	return c.SaveToken()
}

func (c *Client) GetGenres() ([]Genre, error) {
	url := c.BaseURL + "/catalog/genres/?per_page=100"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token.AccessToken)

	resp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get genres: %s", string(body))
	}

	var genreResp GenreResponse
	if err := json.NewDecoder(resp.Body).Decode(&genreResp); err != nil {
		return nil, err
	}

	return genreResp.Results, nil
}

func (c *Client) GetTop100(genreID int) ([]Track, error) {
	// Try the standard top 100 endpoint first
	url := fmt.Sprintf("%s/catalog/genres/%d/top/100?per_page=100", c.BaseURL, genreID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token.AccessToken)

	resp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var trackResp TrackResponse
		if err := json.NewDecoder(resp.Body).Decode(&trackResp); err != nil {
			return nil, err
		}
		return trackResp.Results, nil
	}

	// Fallback to search if the specific endpoint fails (e.g. 404)
	// Note: This is a heuristic fallback.
	searchURL := fmt.Sprintf("%s/catalog/search?q=genre_id:%d&per_page=100&type=tracks", c.BaseURL, genreID)
	req, err = http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token.AccessToken)

	resp, err = c.doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get top 100 (fallback): %s", string(body))
	}

	// Search response structure might be different, usually has 'tracks' key
	var searchResp struct {
		Tracks []Track `json:"tracks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	return searchResp.Tracks, nil
}
