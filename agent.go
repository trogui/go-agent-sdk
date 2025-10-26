package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/rs/zerolog/log"
)

// Config contains the agent configuration
type Config struct {
	APIKey       string
	APIURL       string
	Model        string
	SystemPrompt string
	MaxLoops     int
	Temperature  float64
}

// Tool represents a registered tool
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]Parameter
	Required    []string
	Handler     ToolHandler
}

// Parameter defines a tool parameter
type Parameter struct {
	Type        string
	Description string
	Items       *Items // For array types
}

// Items defines the type of elements in an array
type Items struct {
	Type string
}

// ToolHandler is the function that executes the tool
type ToolHandler func(args json.RawMessage) (any, error)

// Agent is the AI agent
type Agent struct {
	config Config
	tools  map[string]*Tool
	client *http.Client
}

// Response is the agent's response
type Response struct {
	Content      string
	Usage        Usage
	FinishReason string
	LoopCount    int
}

// Usage contains token usage information
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// New creates a new agent
func New(config Config) (*Agent, error) {
	// Checks
	if config.APIURL == "" {
		return nil, fmt.Errorf("API URL is required")
	}
	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	if config.SystemPrompt == "" {
		return nil, fmt.Errorf("system prompt is required")
	}
	if config.MaxLoops == 0 {
		config.MaxLoops = 20
	}

	return &Agent{
		config: config,
		tools:  make(map[string]*Tool),
		client: &http.Client{},
	}, nil
}

// RegisterTool registers a new tool
func (a *Agent) RegisterTool(tool *Tool) {
	a.tools[tool.Name] = tool
}

// Run executes the agent with a prompt
func (a *Agent) Run(prompt string) (*Response, error) {
	messages := []any{
		map[string]string{"role": "system", "content": a.config.SystemPrompt},
		map[string]string{"role": "user", "content": prompt},
	}

	log.Info().Str("prompt", prompt).Msg("[Agent] Starting run")

	reason := ""
	loopCount := 0
	var lastResponse *apiResponse
	var totalUsage Usage

	for reason != "stop" {
		loopCount++

		if loopCount > a.config.MaxLoops {
			return nil, fmt.Errorf("maximum loop iterations (%d) exceeded", a.config.MaxLoops)
		}

		log.Info().Int("iteration", loopCount).Msg("[Agent] Starting iteration")

		resp, err := a.callAPI(messages)
		if err != nil {
			return nil, fmt.Errorf("API call error: %w", err)
		}

		lastResponse = resp
		reason = resp.Choices[0].FinishReason

		// Accumulate token usage from this iteration
		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

		log.Info().
			Int("iteration", loopCount).
			Str("finish_reason", reason).
			Int("num_tool_calls", len(resp.Choices[0].Message.ToolCalls)).
			Msg("[Agent] Received response")

		if reason == "tool_calls" {
			// Add assistant message with tool_calls
			assistantMessage := map[string]any{
				"role":       "assistant",
				"tool_calls": resp.Choices[0].Message.ToolCalls,
			}
			messages = append(messages, assistantMessage)

			// Execute each tool call
			for _, toolCall := range resp.Choices[0].Message.ToolCalls {
				log.Info().
					Str("tool_name", toolCall.Function.Name).
					Str("arguments", toolCall.Function.Arguments).
					Msg("[Agent] Executing tool")

				result, err := a.executeTool(toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments))

				var content string
				if err != nil {
					log.Error().Err(err).Str("tool", toolCall.Function.Name).Msg("[Agent] Tool execution error")
					content = fmt.Sprintf(`{"error": "%s"}`, err.Error())
				} else {
					resultJSON, err := json.Marshal(result)
					if err != nil {
						return nil, fmt.Errorf("error encoding tool result: %w", err)
					}
					content = string(resultJSON)
				}

				// Add tool response
				toolResponse := map[string]string{
					"role":         "tool",
					"content":      content,
					"tool_call_id": toolCall.ID,
				}
				messages = append(messages, toolResponse)
			}
		}
	}

	if len(lastResponse.Choices) == 0 {
		return nil, fmt.Errorf("no response from API")
	}

	return &Response{
		Content:      lastResponse.Choices[0].Message.Content,
		Usage:        totalUsage,
		FinishReason: lastResponse.Choices[0].FinishReason,
		LoopCount:    loopCount,
	}, nil
}

// executeTool executes a registered tool
func (a *Agent) executeTool(name string, args json.RawMessage) (any, error) {
	tool, ok := a.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	return tool.Handler(args)
}

// callAPI calls the API with the url provided in the config
func (a *Agent) callAPI(messages []any) (*apiResponse, error) {
	// Convert tools to API format
	apiTools := make([]apiTool, 0, len(a.tools))
	for _, tool := range a.tools {
		properties := make(map[string]apiParameter)
		for name, param := range tool.Parameters {
			apiParam := apiParameter{
				Type:        param.Type,
				Description: param.Description,
			}
			if param.Items != nil {
				apiParam.Items = &apiItems{Type: param.Items.Type}
			}
			properties[name] = apiParam
		}

		apiTools = append(apiTools, apiTool{
			Type: "function",
			Function: apiFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters: apiParameters{
					Type:       "object",
					Properties: properties,
					Required:   tool.Required,
				},
			},
		})
	}

	requestBody := map[string]any{
		"model":    a.config.Model,
		"messages": messages,
		"tools":    apiTools,
	}

	if a.config.Temperature > 0 {
		requestBody["temperature"] = a.config.Temperature
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("error encoding request: %w", err)
	}

	req, err := http.NewRequest("POST", a.config.APIURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+a.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	return &apiResp, nil
}

// Internal structs for API communication

type apiResponse struct {
	ID      string      `json:"id"`
	Choices []apiChoice `json:"choices"`
	Usage   Usage       `json:"usage"`
}

type apiChoice struct {
	Index        int        `json:"index"`
	Message      apiMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

type apiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []apiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type apiToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function apiFunctionCall `json:"function"`
}

type apiFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type apiTool struct {
	Type     string      `json:"type"`
	Function apiFunction `json:"function"`
}

type apiFunction struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Parameters  apiParameters `json:"parameters"`
}

type apiParameters struct {
	Type       string                  `json:"type"`
	Properties map[string]apiParameter `json:"properties"`
	Required   []string                `json:"required"`
}

type apiParameter struct {
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Items       *apiItems `json:"items,omitempty"`
}

type apiItems struct {
	Type string `json:"type"`
}
