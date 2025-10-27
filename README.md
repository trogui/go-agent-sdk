# AI Agent Package

Lightweight helper to run looping OpenAI-compatible agents with tool support. Features both one-shot execution and interactive multi-turn sessions with persistent conversation history.

## Quick Start

```go
import "github.com/trogui/go-agent-sdk/agent"

ag, err := agent.New(agent.Config{
    APIKey:       os.Getenv("OPENROUTER_API_KEY"),              // required
    APIURL:       "https://openrouter.ai/api/v1/chat/completions", // required
    Model:        "gpt-4o-mini",                                // required
    SystemPrompt: "You are a helpful assistant.",               // required
    MaxLoops:     20,  // optional: defaults to 20
    Temperature:  0,   // optional: defaults to 0
})
```

## Registering Tools

### Single Tool

```go
ag.RegisterTool(&agent.Tool{
    Name:        "searchBooks",
    Description: "Search the catalog by keyword",
    Parameters: map[string]agent.Parameter{
        "query": {
            Type:        "string",
            Description: "Search phrase",
        },
    },
    Required: []string{"query"},
    Handler: func(args json.RawMessage) (any, error) {
        var payload struct {
            Query string `json:"query"`
        }
        if err := json.Unmarshal(args, &payload); err != nil {
            return nil, err
        }
        return search(payload.Query), nil
    },
})
```

### Multiple Tools

To register multiple tools at once, use `RegisterTools`:

```go
ag.RegisterTools(
    &agent.Tool{
        Name:        "searchBooks",
        Description: "Search the catalog by keyword",
        // ... parameters and handler
    },
    &agent.Tool{
        Name:        "getPrice",
        Description: "Get the price of a book",
        // ... parameters and handler
    },
)
```

The handler gets the raw JSON arguments coming from the model. Return any Go value; it will be serialized back to JSON and fed to the model as the tool output.

## Running the Agent

### One-shot execution

For simple tasks that don't require conversation:

```go
resp, err := ag.Run("Find three Dickens books and give me prices")
if err != nil {
    log.Fatal().Err(err).Msg("agent failed")
}
fmt.Println(resp.Content)
fmt.Printf("Tokens: %+v\n", resp.Usage)
```

The agent keeps cycling until the API returns `finish_reason == "stop"` or `MaxLoops` is hit. Every iteration gets logged through zerolog for easy tracing.

## Interactive Sessions

For multi-turn conversations with persistent context, use sessions instead of one-shot `Run()` calls. Sessions maintain full conversation history, allowing the agent to reference previous turns and provide coherent multi-turn interactions:

```go
ctx := context.Background()
session := ag.NewSession(ctx)

// Start first turn
session.Send("Create a Sunday plan for me")

for event := range session.Events() {
    switch event.Type {
    case agent.EventTurnComplete:
        fmt.Println("Agent:", event.Content)
        // Continue conversation - full context is preserved
        session.Send("Change the hiking part to swimming")

    case agent.EventToolCall:
        fmt.Printf("Executing tool: %s\n", event.Content)

    case agent.EventToolResult:
        fmt.Printf("Tool result: %s\n", event.Content)

    case agent.EventError:
        log.Error().Str("error", event.Content).Msg("Agent error")

    case agent.EventIterationStart:
        // Optional: track agent thinking
        fmt.Printf("[Iteration %d]\n", event.Iteration)
    }
}
```

### Session Methods

- `Send(message string)`: Send a message and start a new turn. The conversation history is automatically maintained.
- `SendInput(input string)`: Respond to `EventNeedInput` events (for tool-based user interaction).
- `GetHistory() []any`: Retrieve the full message history of the session.
- `Events() <-chan AgentEvent`: Get the channel for receiving events.
- `Close()`: Close the session and release resources.

### Session Events

| Event Type | Description |
| --- | --- |
| `EventIterationStart` | A new API call iteration is starting |
| `EventToolCall` | The agent is about to execute a tool |
| `EventToolResult` | A tool has completed execution |
| `EventNeedInput` | The agent is requesting user input (via a registered tool) |
| `EventTurnComplete` | The agent has finished a turn (ready for new message) |
| `EventError` | An error occurred |

## Configuration Reference

| Field | Description |
| --- | --- |
| `APIKey` | Required. API key for any OpenAI-compatible server. |
| `APIURL` | Required. Full chat completions endpoint for your OpenAI-compatible gateway. |
| `Model` | Required. Model name understood by your provider. |
| `SystemPrompt` | Required. Prime the assistant with your persona/instructions. |
| `MaxLoops` | Optional. Stops the tool loop after N turns (default 20). |
| `Temperature` | Optional. Defaults to 0. Only sent when > 0 so you control randomness. |

## Tips

- Always validate and sanitize tool arguments before acting on them.
- Return concise JSON from tools; the agent sends it verbatim to the model.
- Use `MaxLoops` to keep long-running tool chains under control.
- Inspect `Response.Usage` for token accounting and to decide whether to stop earlier.(Only woks with Openrouter)
