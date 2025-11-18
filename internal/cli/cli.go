package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	"beatport-top100/beatport"

	"golang.org/x/term"
)

type Config struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func loadConfig() (*Config, error) {
	file, err := os.Open("config.json")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Config doesn't exist, not an error
		}
		return nil, err
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

func saveConfig(username, password string) {
	config := Config{
		Username: username,
		Password: password,
	}
	file, err := os.Create("config.json")
	if err != nil {
		log.Printf("Warning: Failed to create config.json: %v", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "    ")
	if err := encoder.Encode(config); err != nil {
		log.Printf("Warning: Failed to write to config.json: %v", err)
	}
}

func Run() {
	var jsonOutput bool
	var csvOutput bool
	flag.BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	flag.BoolVar(&csvOutput, "csv", false, "Output in CSV format")
	flag.Parse()

	reader := bufio.NewReader(os.Stdin)
	config, err := loadConfig()
	if err != nil {
		log.Printf("Warning: Failed to load config: %v", err)
	}

	var username, password string

	if config != nil && config.Username != "" && config.Password != "" {
		if !jsonOutput && !csvOutput {
			fmt.Println("Using credentials from config.json")
		}
		username = config.Username
		password = config.Password
	} else {
		fmt.Print("Enter Beatport Username: ")
		username, _ = reader.ReadString('\n')
		username = strings.TrimSpace(username)

		fmt.Print("Enter Beatport Password: ")
		bytePassword, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			log.Fatalf("Failed to read password: %v", err)
		}
		password = string(bytePassword)
		fmt.Println() // Print newline after hidden input
	}

	// Ask for Genre
	// If we are in non-interactive mode (e.g. piped input or just flags), we might want to support a flag for genre too.
	// But for now, let's stick to the existing behavior but make it testable if we wanted to inject stdin/stdout.

	// Note: The original code prompted for genre AFTER login.
	// Let's keep the flow.

	client, err := beatport.NewClient()
	if err != nil {
		log.Fatalf("Error creating client: %v", err)
	}

	if !jsonOutput && !csvOutput {
		fmt.Println("Authenticating...")
	}
	if err := client.Login(username, password); err != nil {
		log.Fatalf("Login failed: %v", err)
	}

	// Authorize and get token
	code, err := client.Authorize()
	if err != nil {
		log.Fatalf("Authorization failed: %v", err)
	}

	if err := client.GetToken(code); err != nil {
		log.Fatalf("Token exchange failed: %v", err)
	}

	if !jsonOutput && !csvOutput {
		fmt.Println("Successfully authenticated!")
	}

	// Save config if it was manual entry
	if config == nil || config.Username == "" {
		fmt.Print("Do you want to save credentials to config.json? (y/n): ")
		save, _ := reader.ReadString('\n')
		save = strings.TrimSpace(save)
		if strings.ToLower(save) == "y" {
			saveConfig(username, password)
			fmt.Println("Credentials saved.")
		}
	}

	fmt.Print("Enter Genre (e.g. Techno): ")
	genreName, _ := reader.ReadString('\n')
	genreName = strings.TrimSpace(genreName)

	if !jsonOutput && !csvOutput {
		fmt.Println("Fetching genres...")
	}
	genres, err := client.GetGenres()
	if err != nil {
		log.Fatalf("Error fetching genres: %v", err)
	}

	var selectedGenre *beatport.Genre
	for _, g := range genres {
		if strings.EqualFold(g.Name, genreName) {
			selectedGenre = &g
			break
		}
	}

	if selectedGenre == nil {
		fmt.Printf("Genre '%s' not found. Available genres:\n", genreName)
		for _, g := range genres {
			fmt.Printf("- %s (ID: %d)\n", g.Name, g.ID)
		}
		log.Fatalf("Please choose one of the available genres.")
	}

	if !jsonOutput && !csvOutput {
		fmt.Printf("Fetching Top 100 for %s (ID: %d)...\n", selectedGenre.Name, selectedGenre.ID)
	}
	tracks, err := client.GetTop100(selectedGenre.ID)
	if err != nil {
		log.Fatalf("Error fetching Top 100: %v", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(tracks); err != nil {
			log.Fatalf("Error encoding JSON: %v", err)
		}
		return
	}

	if csvOutput {
		// Simple CSV output
		fmt.Println("Artist,Title,Mix Name")
		for _, track := range tracks {
			artistName := ""
			if len(track.Artists) > 0 {
				artistName = track.Artists[0].Name
			}
			fmt.Printf("%s,%s,%s\n", artistName, track.Name, track.MixName)
		}
		return
	}

	fmt.Println("\nTop 100 Tracks:")
	for i, track := range tracks {
		artistName := ""
		if len(track.Artists) > 0 {
			artistName = track.Artists[0].Name
		}
		fmt.Printf("%d. %s - %s (%s)\n", i+1, artistName, track.Name, track.MixName)
	}
}
