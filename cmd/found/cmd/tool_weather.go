package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	fm "github.com/blacktop/go-foundationmodels"
	"github.com/spf13/cobra"
)

// OpenMeteo response structure
type OpenMeteoResponse struct {
	Current struct {
		Time        string  `json:"time"`
		Temperature float64 `json:"temperature_2m"`
		Humidity    int     `json:"relative_humidity_2m"`
		Pressure    float64 `json:"surface_pressure"`
		WindSpeed   float64 `json:"wind_speed_10m"`
		WindDir     int     `json:"wind_direction_10m"`
		WeatherCode int     `json:"weather_code"`
	} `json:"current"`
	CurrentUnits struct {
		Temperature string `json:"temperature_2m"`
		Humidity    string `json:"relative_humidity_2m"`
		Pressure    string `json:"surface_pressure"`
		WindSpeed   string `json:"wind_speed_10m"`
		WindDir     string `json:"wind_direction_10m"`
	} `json:"current_units"`
}

// Geocoding response structure (using OpenStreetMap Nominatim)
type GeocodingResponse []struct {
	PlaceName string `json:"display_name"`
	Lat       string `json:"lat"`
	Lon       string `json:"lon"`
	Name      string `json:"name"`
	Country   string `json:"country"`
	State     string `json:"state"`
}

// Location represents a geographic location
type Location struct {
	Name    string
	Lat     float64
	Lon     float64
	Country string
	State   string
}

// Define argument definitions for validation
var weatherArgDefs = []fm.ToolArgument{
	{
		Name:        "location",
		Type:        "string",
		Description: "City or location name",
		Required:    true,
	},
}

// WeatherTool fetches weather information from API
type WeatherTool struct{}

func (w *WeatherTool) Name() string {
	return "checkWeather"
}

func (w *WeatherTool) Description() string {
	return "Check current weather conditions"
}

// GetParameters returns the parameter definitions for the weather tool
func (w *WeatherTool) GetParameters() []fm.ToolArgument {
	return weatherArgDefs
}

func (w *WeatherTool) Execute(args map[string]any) (fm.ToolResult, error) {
	locationVal, exists := args["location"]
	if !exists {
		return fm.ToolResult{
			Error: "Missing required argument: location",
		}, nil
	}

	locationStr, ok := locationVal.(string)
	if !ok {
		return fm.ToolResult{
			Error: "Location must be a string",
		}, nil
	}

	// First, geocode the location to get lat/lon
	location, err := geocodeLocation(locationStr)
	if err != nil {
		return fm.ToolResult{
			Error: fmt.Sprintf("Failed to find location: %v", err),
		}, nil
	}

	// Fetch weather data using OpenMeteo
	weatherData, err := fetchOpenMeteoWeather(location.Lat, location.Lon)
	if err != nil {
		return fm.ToolResult{
			Error: fmt.Sprintf("Failed to fetch weather data: %v", err),
		}, nil
	}

	// Convert temperature to Fahrenheit
	tempF := weatherData.Current.Temperature*9/5 + 32

	// Get weather condition from code
	condition := getWeatherCondition(weatherData.Current.WeatherCode)

	// Get wind direction
	windDir := getWindDirection(weatherData.Current.WindDir)

	// Convert wind speed from km/h to mph
	windMph := weatherData.Current.WindSpeed * 0.621371

	// Format weather information
	weatherInfo := fmt.Sprintf(`Current conditions for %s:
Temperature: %.1f°F (%.1f°C)
Condition: %s
Humidity: %d%%
Wind: %.1f mph %s
Pressure: %.1f hPa
Last updated: %s`,
		location.Name,
		tempF,
		weatherData.Current.Temperature,
		condition,
		weatherData.Current.Humidity,
		windMph,
		windDir,
		weatherData.Current.Pressure,
		weatherData.Current.Time)

	return fm.ToolResult{
		Content: weatherInfo,
	}, nil
}

// geocodeLocation converts a location string to lat/lon using OpenStreetMap Nominatim
func geocodeLocation(location string) (*Location, error) {
	// URL encode the location
	encodedLocation := url.QueryEscape(location)

	// Use OpenStreetMap Nominatim API (free, no API key required)
	apiURL := fmt.Sprintf("https://nominatim.openstreetmap.org/search?q=%s&format=json&limit=1", encodedLocation)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to geocode location: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("geocoding API request failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read geocoding response: %v", err)
	}

	var geoResponse GeocodingResponse
	if err := json.Unmarshal(body, &geoResponse); err != nil {
		return nil, fmt.Errorf("failed to parse geocoding response: %v", err)
	}

	if len(geoResponse) == 0 {
		return nil, fmt.Errorf("location not found: %s", location)
	}

	// Parse lat/lon from strings
	lat, err := strconv.ParseFloat(geoResponse[0].Lat, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid latitude: %v", err)
	}

	lon, err := strconv.ParseFloat(geoResponse[0].Lon, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid longitude: %v", err)
	}

	return &Location{
		Name:    geoResponse[0].Name,
		Lat:     lat,
		Lon:     lon,
		Country: geoResponse[0].Country,
		State:   geoResponse[0].State,
	}, nil
}

// fetchOpenMeteoWeather fetches weather data from OpenMeteo API
func fetchOpenMeteoWeather(lat, lon float64) (*OpenMeteoResponse, error) {
	// OpenMeteo API URL with current weather
	apiURL := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.6f&longitude=%.6f&current=temperature_2m,relative_humidity_2m,surface_pressure,wind_speed_10m,wind_direction_10m,weather_code&timezone=auto", lat, lon)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch weather data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather API request failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read weather response: %v", err)
	}

	var weatherResponse OpenMeteoResponse
	if err := json.Unmarshal(body, &weatherResponse); err != nil {
		return nil, fmt.Errorf("failed to parse weather response: %v", err)
	}

	return &weatherResponse, nil
}

// getWeatherCondition converts OpenMeteo weather code to readable condition
func getWeatherCondition(code int) string {
	switch code {
	case 0:
		return "Clear sky"
	case 1:
		return "Mainly clear"
	case 2:
		return "Partly cloudy"
	case 3:
		return "Overcast"
	case 45, 48:
		return "Foggy"
	case 51, 53, 55:
		return "Drizzle"
	case 56, 57:
		return "Freezing drizzle"
	case 61, 63, 65:
		return "Rain"
	case 66, 67:
		return "Freezing rain"
	case 71, 73, 75:
		return "Snow"
	case 77:
		return "Snow grains"
	case 80, 81, 82:
		return "Rain showers"
	case 85, 86:
		return "Snow showers"
	case 95:
		return "Thunderstorm"
	case 96, 99:
		return "Thunderstorm with hail"
	default:
		return "Unknown"
	}
}

// getWindDirection converts wind direction degrees to compass direction
func getWindDirection(degrees int) string {
	directions := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	index := int((float64(degrees) + 11.25) / 22.5)
	return directions[index%16]
}

// weatherCmd represents the weather command
var weatherCmd = &cobra.Command{
	Use:   "weather [location]",
	Short: "Get weather information with emoji-filled responses",
	Long: `Get current weather information for any location using Foundation Models.
Provides real-time weather data including temperature, conditions, humidity, and wind.
The assistant will respond with friendly, informative weather descriptions.`,
	Example: `  # Get weather for cities
  found tool weather "New York, NY"
  found tool weather "London, UK"
  found tool weather "Tokyo, Japan"
  found tool weather "Paris, France"

  # Various location formats
  found tool weather "San Francisco"
  found tool weather "Berlin, Germany"
  found tool weather "Sydney, Australia"

  # Test Go tool directly (bypass Foundation Models)
  found tool weather --direct "New York"`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		location := args[0]

		// Setup slog based on verbose flag
		verbose, _ := cmd.Flags().GetBool("verbose")
		SetupSlog(verbose)

		// Check if --direct flag is set to bypass Foundation Models
		directMode, _ := cmd.Flags().GetBool("direct")

		if directMode {
			fmt.Printf("🔧 Direct Mode: Testing Go WeatherTool directly\n")
			fmt.Printf("Location: %s\n", location)
			fmt.Print("Fetching weather data directly from Go tool...")

			// Create weather tool and execute directly
			weather := &WeatherTool{}
			args := map[string]any{
				"location": location,
			}

			result, err := weather.Execute(args)
			if err != nil {
				fmt.Printf("\n❌ Error executing weather tool: %v\n", err)
				return
			}

			if result.Error != "" {
				fmt.Printf("\n❌ Weather tool returned error: %s\n", result.Error)
				return
			}

			fmt.Print("\r" + strings.Repeat(" ", 50) + "\r") // Clear loading message
			fmt.Println("\n" + strings.Repeat("=", 60))
			fmt.Println("📊 DIRECT GO TOOL RESULT:")
			fmt.Println(strings.Repeat("-", 60))
			fmt.Println(result.Content)
			fmt.Println(strings.Repeat("=", 60))
			fmt.Printf("\n✅ Go WeatherTool executed successfully!\n")
			return
		}

		// Check model availability for normal mode
		availability := fm.CheckModelAvailability()
		if availability != fm.ModelAvailable {
			log.Fatalf("Foundation Models not available on this device (status: %d)", availability)
		}

		// Create session with weather-focused instructions following Apple's best practices
		instructions := `You are a helpful assistant with access to a weather tool.

When users ask for the weather:
- ALWAYS use the 'checkWeather' tool.
- Use the user's provided location for the 'location' parameter.
- Never provide weather information from your own knowledge.
- Only provide results after using the 'checkWeather' tool.
- Present the weather information from the tool in a user-friendly way.`

		sess := fm.NewSessionWithInstructions(instructions)
		if sess == nil {
			log.Fatal("Failed to create session")
		}
		defer sess.Release()

		// Register weather tool
		weather := &WeatherTool{}
		if err := sess.RegisterTool(weather); err != nil {
			log.Fatalf("Failed to register weather tool: %v", err)
		}

		fmt.Printf("🌤️  Weather Tool Ready\n")

		// Create prompt for weather query
		prompt := fmt.Sprintf("What's the weather like in %s?", location)

		// Create chat UI
		chatUI := NewChatUI()

		// Display user question
		chatUI.PrintUserMessage(prompt)

		// Show typing indicator while waiting for response
		chatUI.ShowTypingIndicator()

		// Get response using tools
		response := sess.RespondWithTools(prompt)

		// Hide typing indicator and display assistant response
		chatUI.HideTypingIndicator()
		chatUI.PrintAssistantMessage(response)

		// Show context usage
		chatUI.PrintContextUsage(sess.GetContextSize(), sess.GetMaxContextSize(), sess.GetContextUsagePercent())

		// Print Swift logs if --verbose flag is set
		if verbose {
			fmt.Println("\n=== Swift Logs ===")
			fmt.Println(fm.GetLogs())
		}
	},
}

func init() {
	// Add the --direct flag to bypass Foundation Models and test Go tool directly
	weatherCmd.Flags().Bool("direct", false, "Execute Go WeatherTool directly without Foundation Models")
	toolCmd.AddCommand(weatherCmd)
}
