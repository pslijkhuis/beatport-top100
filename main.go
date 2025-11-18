package main

import (
	"bufio"
	"encoding/json"
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

func main() {
	reader := bufio.NewReader(os.Stdin)
	config, err := loadConfig()
	if err != nil {
		log.Printf("Warning: Failed to load config: %v", err)
	}

	var username, password string

	if config != nil && config.Username != "" && config.Password != "" {
		fmt.Println("Using credentials from config.json")
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
		fmt.Println() // Newline after password input

		fmt.Print("Do you want to save these credentials to config.json? (y/n): ")
		save, _ := reader.ReadString('\n')
		save = strings.TrimSpace(save)
		if strings.ToLower(save) == "y" {
			newConfig := Config{Username: username, Password: password}
			file, err := os.Create("config.json")
			if err != nil {
				log.Printf("Warning: Failed to create config.json: %v", err)
			} else {
				defer file.Close()
				encoder := json.NewEncoder(file)
				encoder.SetIndent("", "    ")
				if err := encoder.Encode(newConfig); err != nil {
					log.Printf("Warning: Failed to write to config.json: %v", err)
				} else {
					fmt.Println("Credentials saved to config.json")
				}
			}
		}
	}

	fmt.Print("Enter Genre (e.g. Techno): ")
	genreName, _ := reader.ReadString('\n')
	genreName = strings.TrimSpace(genreName)

	client, err := beatport.NewClient()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	fmt.Println("Authenticating...")
	if err := client.Login(username, password); err != nil {
		log.Fatalf("Login failed: %v", err)
	}

	code, err := client.Authorize()
	if err != nil {
		log.Fatalf("Authorization failed: %v", err)
	}

	if err := client.GetToken(code); err != nil {
		log.Fatalf("Failed to get token: %v", err)
	}
	fmt.Println("Successfully authenticated!")

	fmt.Println("Fetching genres...")
	genres, err := client.GetGenres()
	if err != nil {
		log.Fatalf("Failed to fetch genres: %v", err)
	}

	var genreID int
	found := false
	for _, g := range genres {
		if strings.EqualFold(g.Name, genreName) {
			genreID = g.ID
			found = true
			break
		}
	}

	if !found {
		fmt.Printf("Genre '%s' not found. Available genres:\n", genreName)
		for _, g := range genres {
			fmt.Printf("- %s (ID: %d)\n", g.Name, g.ID)
		}
		log.Fatalf("Please choose one of the available genres.")
	}

	fmt.Printf("Fetching Top 100 for %s (ID: %d)...\n", genreName, genreID)
	tracks, err := client.GetTop100(genreID)
	if err != nil {
		log.Fatalf("Failed to fetch top 100: %v", err)
	}

	fmt.Println("\nTop 100 Tracks:")
	for i, track := range tracks {
		var artists []string
		for _, a := range track.Artists {
			artists = append(artists, a.Name)
		}
		artistStr := strings.Join(artists, ", ")
		fmt.Printf("%d. %s - %s (%s)\n", i+1, artistStr, track.Name, track.MixName)
	}
}
