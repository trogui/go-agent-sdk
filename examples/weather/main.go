package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/rs/zerolog"
	"github.com/trogui/go-agent-sdk/agent"
)

// WeatherDB simulates a weather database
type WeatherDB struct {
	temperatures map[string]float64
	conditions   map[string]string
}

// NewWeatherDB creates a new weather database
func NewWeatherDB() *WeatherDB {
	return &WeatherDB{
		temperatures: map[string]float64{
			"new_york":   15.5,
			"london":     12.0,
			"tokyo":      22.3,
			"sydney":     25.8,
			"paris":      14.2,
		},
		conditions: map[string]string{
			"new_york": "Cloudy",
			"london":   "Rainy",
			"tokyo":    "Sunny",
			"sydney":   "Clear",
			"paris":    "Partly Cloudy",
		},
	}
}

// GetWeather returns weather information for a city
func (db *WeatherDB) GetWeather(city string) map[string]interface{} {
	temp, tempOk := db.temperatures[city]
	condition, condOk := db.conditions[city]

	if !tempOk || !condOk {
		return map[string]interface{}{
			"error": fmt.Sprintf("Weather data not found for city: %s", city),
		}
	}

	return map[string]interface{}{
		"city":        city,
		"temperature": temp,
		"condition":   condition,
		"unit":        "Celsius",
	}
}

// GetMultipleCities returns weather for multiple cities
func (db *WeatherDB) GetMultipleCities(cities []string) []map[string]interface{} {
	results := []map[string]interface{}{}
	for _, city := range cities {
		results = append(results, db.GetWeather(city))
	}
	return results
}

func main() {
	// Set up logging
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Create weather database
	weatherDB := NewWeatherDB()

	// Get API credentials
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY environment variable is required")
	}

	// Create agent
	ag, err := agent.New(agent.Config{
		APIKey:       apiKey,
		APIURL:       "https://openrouter.ai/api/v1/chat/completions",
		Model:        "gpt-4o-mini",
		SystemPrompt: "You are a weather information assistant. Use the available tools to provide weather information for cities. Be concise and helpful. Always provide the information in a clear format.",
		MaxLoops:     10,
		Temperature:  0.7,
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Register weather tools
	ag.RegisterTools(
		&agent.Tool{
			Name:        "get_weather",
			Description: "Get weather information for a specific city",
			Parameters: map[string]agent.Parameter{
				"city": {
					Type:        "string",
					Description: "The city name (lowercase, e.g., 'new_york', 'london', 'tokyo')",
				},
			},
			Required: []string{"city"},
			Handler: func(args json.RawMessage) (any, error) {
				var payload struct {
					City string `json:"city"`
				}
				if err := json.Unmarshal(args, &payload); err != nil {
					return nil, err
				}

				return weatherDB.GetWeather(payload.City), nil
			},
		},
		&agent.Tool{
			Name:        "get_multiple_cities",
			Description: "Get weather information for multiple cities at once",
			Parameters: map[string]agent.Parameter{
				"cities": {
					Type:        "array",
					Description: "Array of city names",
					Items: &agent.Items{
						Type: "string",
					},
				},
			},
			Required: []string{"cities"},
			Handler: func(args json.RawMessage) (any, error) {
				var payload struct {
					Cities []string `json:"cities"`
				}
				if err := json.Unmarshal(args, &payload); err != nil {
					return nil, err
				}

				return map[string]interface{}{
					"cities": weatherDB.GetMultipleCities(payload.Cities),
				}, nil
			},
		},
	)

	ctx := context.Background()

	// Example 1: Simple single-turn request using agent.Run()
	fmt.Println("=== Weather Agent Example - agent.Run() ===\n")

	prompt := "What is the weather in Tokyo and Paris right now?"
	fmt.Printf("Prompt: %s\n", prompt)
	fmt.Println("---")

	response, err := ag.Run(prompt)
	if err != nil {
		log.Fatalf("Failed to run agent: %v", err)
	}

	fmt.Printf("Response: %s\n\n", response.Content)
	fmt.Printf("Tokens used - Prompt: %d, Completion: %d, Total: %d\n",
		response.Usage.PromptTokens,
		response.Usage.CompletionTokens,
		response.Usage.TotalTokens)
	fmt.Printf("Loops executed: %d\n", response.LoopCount)
	fmt.Printf("Finish reason: %s\n", response.FinishReason)

	// Example 2: Another single-turn request
	fmt.Println("\n" + "---\n")

	prompt2 := "Compare the weather in London, Sydney, and New York"
	fmt.Printf("Prompt: %s\n", prompt2)
	fmt.Println("---")

	response2, err := ag.Run(prompt2)
	if err != nil {
		log.Fatalf("Failed to run agent: %v", err)
	}

	fmt.Printf("Response: %s\n\n", response2.Content)
	fmt.Printf("Tokens used - Prompt: %d, Completion: %d, Total: %d\n",
		response2.Usage.PromptTokens,
		response2.Usage.CompletionTokens,
		response2.Usage.TotalTokens)
	fmt.Printf("Loops executed: %d\n", response2.LoopCount)

	_ = ctx // Use context even though we're not using it explicitly
}
