package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/trogui/go-agent-sdk/agent"
)

const tasksFile = "tasks.json"

// Task represents a to-do item
type Task struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
	CreatedAt string `json:"created_at"`
}

// TaskDB manages task persistence
type TaskDB struct {
	nextID int
	tasks  []Task
}

// LoadTasks loads tasks from JSON file
func LoadTasks() *TaskDB {
	db := &TaskDB{
		nextID: 1,
		tasks:  []Task{},
	}

	data, err := os.ReadFile(tasksFile)
	if err != nil {
		return db // File doesn't exist yet
	}

	json.Unmarshal(data, &db.tasks)

	// Calculate next ID
	for _, t := range db.tasks {
		if t.ID >= db.nextID {
			db.nextID = t.ID + 1
		}
	}

	return db
}

// SaveTasks saves tasks to JSON file
func (db *TaskDB) SaveTasks() error {
	data, err := json.MarshalIndent(db.tasks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tasksFile, data, 0644)
}

// AddTask adds a new task
func (db *TaskDB) AddTask(title string) Task {
	task := Task{
		ID:        db.nextID,
		Title:     title,
		Completed: false,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	db.nextID++
	db.tasks = append(db.tasks, task)
	db.SaveTasks()
	return task
}

// CompleteTask marks a task as done
func (db *TaskDB) CompleteTask(id int) (Task, bool) {
	for i, t := range db.tasks {
		if t.ID == id {
			db.tasks[i].Completed = true
			db.SaveTasks()
			return db.tasks[i], true
		}
	}
	return Task{}, false
}

// GetStats returns task statistics
func (db *TaskDB) GetStats() map[string]interface{} {
	total := len(db.tasks)
	completed := 0
	for _, t := range db.tasks {
		if t.Completed {
			completed++
		}
	}

	return map[string]interface{}{
		"total":      total,
		"completed":  completed,
		"pending":    total - completed,
		"percentage": fmt.Sprintf("%.0f%%", float64(completed)*100/float64(total+1)),
	}
}

// GetTasksList returns formatted task list
func (db *TaskDB) GetTasksList() string {
	if len(db.tasks) == 0 {
		return "No tasks yet."
	}

	var result strings.Builder
	for _, t := range db.tasks {
		status := "☐"
		if t.Completed {
			status = "☑"
		}
		result.WriteString(fmt.Sprintf("%s [ID:%d] %s\n", status, t.ID, t.Title))
	}
	return result.String()
}

func main() {
	// Set up logging
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Load tasks database
	db := LoadTasks()

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
		SystemPrompt: "You are a task management assistant. Help the user add, complete, and view the status of their tasks. Be concise and helpful.",
		MaxLoops:     10,
		Temperature:  0.7,
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Register tool: add_task
	ag.RegisterTool(&agent.Tool{
		Name:        "add_task",
		Description: "Add a new task to the list",
		Parameters: map[string]agent.Parameter{
			"title": {
				Type:        "string",
				Description: "Task description",
			},
		},
		Required: []string{"title"},
		Handler: func(args json.RawMessage) (any, error) {
			var payload struct {
				Title string `json:"title"`
			}
			if err := json.Unmarshal(args, &payload); err != nil {
				return nil, err
			}

			task := db.AddTask(payload.Title)
			return map[string]any{
				"id":     task.ID,
				"title":  task.Title,
				"status": "created",
			}, nil
		},
	})

	// Register tool: list_tasks
	ag.RegisterTool(&agent.Tool{
		Name:        "list_tasks",
		Description: "Show all tasks",
		Parameters:  map[string]agent.Parameter{},
		Required:    []string{},
		Handler: func(args json.RawMessage) (any, error) {
			return map[string]interface{}{
				"tasks": db.GetTasksList(),
			}, nil
		},
	})

	// Register tool: complete_task
	ag.RegisterTool(&agent.Tool{
		Name:        "complete_task",
		Description: "Mark a task as completed",
		Parameters: map[string]agent.Parameter{
			"id": {
				Type:        "integer",
				Description: "Task ID",
			},
		},
		Required: []string{"id"},
		Handler: func(args json.RawMessage) (any, error) {
			var payload struct {
				ID int `json:"id"`
			}
			if err := json.Unmarshal(args, &payload); err != nil {
				return nil, err
			}

			task, found := db.CompleteTask(payload.ID)
			if !found {
				return map[string]string{
					"error": fmt.Sprintf("Task with ID %d not found", payload.ID),
				}, nil
			}

			return map[string]interface{}{
				"id":     task.ID,
				"title":  task.Title,
				"status": "completed",
			}, nil
		},
	})

	// Register tool: get_stats
	ag.RegisterTool(&agent.Tool{
		Name:        "get_stats",
		Description: "Get task statistics",
		Parameters:  map[string]agent.Parameter{},
		Required:    []string{},
		Handler: func(args json.RawMessage) (any, error) {
			return db.GetStats(), nil
		},
	})

	// Start interactive session
	ctx := context.Background()
	session := ag.NewSession(ctx)

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n=== Task Manager Agent ===")
	fmt.Println("Type your commands to manage tasks (type 'exit' to quit)\n")

	// First prompt
	fmt.Print("You: ")
	firstMsg, _ := reader.ReadString('\n')
	firstMsg = strings.TrimSpace(firstMsg)

	if firstMsg == "" {
		firstMsg = "Show me my current tasks"
		fmt.Printf("[Using default: %s]\n\n", firstMsg)
	}

	if firstMsg == "exit" {
		return
	}

	fmt.Printf("User: %s\n", firstMsg)
	session.Send(firstMsg)

	// Process events
	for event := range session.Events() {
		switch event.Type {
		case agent.EventIterationStart:
			fmt.Printf("[Iteration %d]\n", event.Iteration)

		case agent.EventToolCall:
			fmt.Printf("  > Calling: %s\n", event.Content)

		case agent.EventToolResult:
			result := truncate(event.Content, 120)
			fmt.Printf("  < Result: %s\n", result)

		case agent.EventTurnComplete:
			fmt.Printf("\nAgent: %s\n\n", event.Content)

			// Ask for next message
			fmt.Print("You: ")
			nextMsg, _ := reader.ReadString('\n')
			nextMsg = strings.TrimSpace(nextMsg)

			if nextMsg == "" || nextMsg == "exit" {
				fmt.Println("\nGoodbye!")
				session.Close()
				return
			}

			fmt.Printf("User: %s\n", nextMsg)
			session.Send(nextMsg)

		case agent.EventError:
			fmt.Printf("\nAgent Error: %s\n", event.Content)
			session.Close()
			return
		}
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
