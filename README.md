# AI Agent Package

Lightweight helper to run looping OPENAI-compatible agents with tool support.

## Quick Start

```go
import "agent-test/aiagent"

agent := aiagent.New(aiagent.Config{
    APIKey:       os.Getenv("OPENROUTER_API_KEY"),              // required
    APIURL:       "https://openrouter.ai/api/v1/chat/completions", // required
    Model:        "gpt-4o-mini",                                // required
    SystemPrompt: "You are a helpful assistant.",               // required
    MaxLoops:     20,  // optional: defaults to 20
    Temperature:  0,   // optional: defaults to 0
})
```

## Registering Tools

```go
agent.RegisterTool(&aiagent.Tool{
    Name:        "searchBooks",
    Description: "Search the catalog by keyword",
    Parameters: map[string]aiagent.Parameter{
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

The handler gets the raw JSON arguments coming from the model. Return any Go value; it will be serialized back to JSON and fed to the model as the tool output.

## Running the Agent

```go
resp, err := agent.Run("Find three Dickens books and give me prices")
if err != nil {
    log.Fatal().Err(err).Msg("agent failed")
}
fmt.Println(resp.Content)
fmt.Printf("Tokens: %+v\n", resp.Usage)
```

The agent keeps cycling until the API returns `finish_reason == "stop"` or `MaxLoops` is hit. Every iteration gets logged through zerolog for easy tracing.

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
- Inspect `Response.Usage` for token accounting and to decide whether to stop earlier.
