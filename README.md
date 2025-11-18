# Beatport Top 100 Fetcher

A Go application that authenticates with Beatport (using the same method as `beets-beatport4`) and fetches the Top 100 tracks for a specified genre.

## Features

-   **Authentication**: Automatically scrapes the necessary client ID and performs the OAuth flow using your Beatport username and password.
-   **Top 100**: Fetches the Top 100 tracks for any given genre.
-   **Smart Fallback**: If the specific Top 100 endpoint fails, it falls back to a search query.
-   **Config Support**: Stores your credentials securely in `config.json` to avoid repeated entry.
-   **Interactive CLI**: Easy-to-use command-line interface.

## Installation

### Prerequisites

-   [Go](https://go.dev/dl/) (1.16 or later)

### Build from Source

1.  Clone the repository:
    ```bash
    git clone https://github.com/pslijkhuis/beatport-top100.git
    cd beatport-top100
    ```

2.  Build the application:
    ```bash
    go build -o beatport-app main.go
    ```

## Usage

1.  Run the application:
    ```bash
    ./beatport-app
    ```
    Or run directly with Go:
    ```bash
    go run main.go
    ```

2.  **First Run**:
    -   Enter your **Beatport Username**.
    -   Enter your **Beatport Password**.
    -   You will be asked if you want to save these credentials to `config.json`.

3.  **Fetch Top 100**:
    -   Enter the **Genre** name (e.g., `Techno`, `Tech House`, `Drum & Bass`).
    -   The app will display the Top 100 tracks for that genre.

## Configuration

The application looks for a `config.json` file in the current directory. You can create it manually or let the app generate it for you.

**Format:**
```json
{
    "username": "your_username",
    "password": "your_password"
}
```

## License

[MIT](LICENSE)
