package beatport

type OAuthToken struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Artist struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Track struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Artists []Artist `json:"artists"`
	MixName string   `json:"mix_name"`
}

type GenreResponse struct {
	Results []Genre `json:"results"`
}

type TrackResponse struct {
	Results []Track `json:"results"`
}
