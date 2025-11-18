package beatport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
)

const (
	APIBaseURL  = "https://api.beatport.com/v4"
	AuthBaseURL = "https://api.beatport.com/v4/auth"
)

type Client struct {
	HTTPClient *http.Client
	Token      *OAuthToken
	ClientID   string
}

func NewClient() (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		HTTPClient: &http.Client{
			Jar: jar,
		},
	}, nil
}

func (c *Client) FetchClientID() error {
	resp, err := c.HTTPClient.Get("https://api.beatport.com/v4/docs/")
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
		scriptURL := "https://api.beatport.com" + match[1]
		scriptResp, err := c.HTTPClient.Get(scriptURL)
		if err != nil {
			continue
		}
		defer scriptResp.Body.Close()

		scriptBody, err := io.ReadAll(scriptResp.Body)
		if err != nil {
			continue
		}

		// Find client_id
		reClientID := regexp.MustCompile(`API_CLIENT_ID:\s*['"](.*?)['"]`)
		clientMatch := reClientID.FindStringSubmatch(string(scriptBody))
		if len(clientMatch) > 1 {
			c.ClientID = clientMatch[1]
			return nil
		}
	}

	return fmt.Errorf("could not fetch API_CLIENT_ID")
}

func (c *Client) Login(username, password string) error {
	loginURL := AuthBaseURL + "/login/"
	data := map[string]string{
		"username": username,
		"password": password,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Post(loginURL, "application/json", bytes.NewBuffer(jsonData))
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
	if c.ClientID == "" {
		if err := c.FetchClientID(); err != nil {
			return "", err
		}
	}

	redirectURI := AuthBaseURL + "/o/post-message/"
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", c.ClientID)
	params.Set("redirect_uri", redirectURI)

	authURL := AuthBaseURL + "/o/authorize/?" + params.Encode()

	// We need to prevent redirects to capture the Location header
	c.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	defer func() { c.HTTPClient.CheckRedirect = nil }()

	resp, err := c.HTTPClient.Get(authURL)
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
	tokenURL := AuthBaseURL + "/o/token/"
	redirectURI := AuthBaseURL + "/o/post-message/"

	data := url.Values{}
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", c.ClientID)

	resp, err := c.HTTPClient.PostForm(tokenURL, data)
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
	return nil
}

func (c *Client) GetGenres() ([]Genre, error) {
	url := APIBaseURL + "/catalog/genres/?per_page=100"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token.AccessToken)

	resp, err := c.HTTPClient.Do(req)
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
	url := fmt.Sprintf("%s/catalog/genres/%d/top/100?per_page=100", APIBaseURL, genreID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token.AccessToken)

	resp, err := c.HTTPClient.Do(req)
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
	searchURL := fmt.Sprintf("%s/catalog/search?q=genre_id:%d&per_page=100&type=tracks", APIBaseURL, genreID)
	req, err = http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token.AccessToken)

	resp, err = c.HTTPClient.Do(req)
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
