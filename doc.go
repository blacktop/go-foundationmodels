/*
Package fm provides a pure Go wrapper around macOS Foundation Models framework.

Foundation Models is Apple's on-device large language model framework introduced in macOS 26 Tahoe,
providing privacy-focused AI capabilities without requiring internet connectivity.

# Features

• Text generation with LanguageModelSession
• Generation options for temperature, max tokens, and other parameters
• Dynamic tool calling with custom Go tools and input validation (⚠️ BETA - currently under development)
• Structured output generation with JSON formatting
• Context window management (4096 token limit)
• Context cancellation and timeout support
• Session lifecycle management with proper memory handling
• System instructions support
• Streaming responses (via Swift shim)

# Requirements

• macOS 26 Tahoe or later
• Apple Intelligence enabled
• Compatible Apple Silicon device

# Basic Usage

Create a session and generate text:

	sess := fm.NewSession()
	defer sess.Release()

	response := sess.Respond("Tell me about artificial intelligence", nil)
	fmt.Println(response)

# Generation Options

Control output with GenerationOptions:

	// Deterministic output
	response := sess.Respond("What is 2+2?", fm.WithDeterministic())

	// Creative output
	response = sess.Respond("Write a story", fm.WithCreative())

	// Custom options
	options := &fm.GenerationOptions{
		Temperature: &[]float32{0.3}[0],
		MaxTokens:   &[]int{100}[0],
	}
	response = sess.Respond("Explain AI", options)

# System Instructions

Create a session with specific behavior:

	instructions := "You are a helpful assistant that responds concisely."
	sess := fm.NewSessionWithInstructions(instructions)
	defer sess.Release()

	response := sess.Respond("What is machine learning?", nil)
	fmt.Println(response)

# Context Management

Foundation Models has a strict 4096 token context window. Monitor usage:

	fmt.Printf("Context: %d/%d tokens (%.1f%% used)\n",
		sess.GetContextSize(), sess.GetMaxContextSize(), sess.GetContextUsagePercent())

	if sess.IsContextNearLimit() {
		// Refresh session when approaching limit
		newSess := sess.RefreshSession()
		sess.Release()
		sess = newSess
	}

# Tool Calling (⚠️ BETA - Under Development)

⚠️ WARNING: Tool calling is currently not working as expected with Apple's Foundation Models.
While the API and validation infrastructure is complete, Foundation Models may not
actually invoke tools even when registered. This is a beta feature under active development.

Define custom tools that the model can call:

	type CalculatorTool struct{}

	func (c *CalculatorTool) Name() string {
		return "calculator"
	}

	func (c *CalculatorTool) Description() string {
		return "Performs basic arithmetic operations"
	}

	func (c *CalculatorTool) Execute(args map[string]any) (fm.ToolResult, error) {
		a := args["a"].(float64)
		b := args["b"].(float64)
		operation := args["operation"].(string)

		var result float64
		switch operation {
		case "add":
			result = a + b
		case "multiply":
			result = a * b
		// ... other operations
		}

		return fm.ToolResult{
			Content: fmt.Sprintf("%.2f", result),
		}, nil
	}

# Tool Input Validation

Add validation to your tools for better error handling:

	// Define validation rules
	var calculatorArgDefs = []fm.ToolArgument{
		{
			Name:     "a",
			Type:     "number",
			Required: true,
		},
		{
			Name:     "b",
			Type:     "number",
			Required: true,
		},
		{
			Name:     "operation",
			Type:     "string",
			Required: true,
			Enum:     []any{"add", "subtract", "multiply", "divide"},
		},
	}

	// Implement ValidatedTool interface
	func (c *CalculatorTool) ValidateArguments(args map[string]any) error {
		return fm.ValidateToolArguments(args, calculatorArgDefs)
	}

Register and use tools:

	calculator := &CalculatorTool{}
	sess.RegisterTool(calculator)

	// Note: Tool calling may not work reliably in current Foundation Models beta
	response := sess.RespondWithTools("What is 15 + 27?")
	fmt.Println(response)

# Structured Output

Generate structured JSON responses:

	response := sess.RespondWithStructuredOutput("Analyze this text: 'Hello world'")
	fmt.Println(response) // Returns formatted JSON

# Context Cancellation

Cancel long-running requests with context support:

	import (
		"context"
		"time"
	)

	// Timeout cancellation
	response, err := sess.RespondWithTimeout(5*time.Second, "Long prompt", nil)
	if err != nil {
		fmt.Printf("Request timed out: %v\n", err)
	}

	// Manual cancellation
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(2 * time.Second)
		cancel()
	}()

	response, err = sess.RespondWithContext(ctx, "Prompt", fm.WithCreative())
	if err != nil {
		fmt.Printf("Request cancelled: %v\n", err)
	}

	// Tool calling with timeout (Note: may not work reliably in current beta)
	response, err = sess.RespondWithToolsTimeout(10*time.Second, "Use calculator")
	if err != nil {
		fmt.Printf("Tool request timed out: %v\n", err)
	}

# Model Availability

Check if Foundation Models is available:

	availability := fm.CheckModelAvailability()
	switch availability {
	case fm.ModelAvailable:
		fmt.Println("✅ Foundation Models available")
	case fm.ModelUnavailableAINotEnabled:
		fmt.Println("❌ Apple Intelligence not enabled")
	case fm.ModelUnavailableDeviceNotEligible:
		fmt.Println("❌ Device not eligible")
	default:
		fmt.Println("❌ Unknown availability status")
	}

# Error Handling

The package provides comprehensive error handling:

	if err := sess.RegisterTool(myTool); err != nil {
		log.Fatalf("Failed to register tool: %v", err)
	}

	// Context validation
	response := sess.Respond(veryLongPrompt, nil)
	if strings.HasPrefix(response, "Error:") {
		fmt.Printf("Request failed: %s\n", response)
	}

	// Context-aware error handling
	import "errors"

	response, err := sess.RespondWithTimeout(30*time.Second, prompt, nil)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Println("Request timed out")
		} else if errors.Is(err, context.Canceled) {
			fmt.Println("Request was cancelled")
		}
	}

# Memory Management

Always release sessions to prevent memory leaks:

	sess := fm.NewSession()
	defer sess.Release() // Important: release session

	// Use session...

# Performance Considerations

• Foundation Models runs entirely on-device
• No internet connection required
• Processing time depends on prompt complexity and device capabilities
• Context window is limited to 4096 tokens
• Token estimation is approximate (4 chars per token)
• Use context cancellation for long-running requests
• Input validation prevents runtime errors and improves performance

# Threading

The package is not thread-safe. Use appropriate synchronization when accessing
sessions from multiple goroutines. Context cancellation is goroutine-safe and can
be used from any goroutine.

# Swift Shim

This package automatically manages the Swift shim library (libFMShim.dylib) that bridges
Foundation Models APIs to C functions callable from Go via purego.

The library search strategy:
1. Look for existing libFMShim.dylib in current directory and common paths
2. If not found, automatically extract embedded library to temp directory
3. Load the library and initialize the Foundation Models interface

No manual setup required - the package is fully self-contained!

# Limitations

• Foundation Models API is still evolving
• Some advanced GenerationOptions may not be fully supported yet
• Tool calling is currently not working reliably (beta feature under development)
• Streaming support is limited
• Context cancellation cannot interrupt actual model computation
• macOS 26 Tahoe only

# License

See LICENSE file for details.
*/
package fm
