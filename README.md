<p align="center">
  <picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/logo-dark.webp" height="400">
  <source media="(prefers-color-scheme: light)" srcset="docs/logo-light.webppng" height="400">
  <img alt="Fallback logo" src="docs/logo-dark.webp" height="300">
</picture>

  <h1 align="center">go-foundationmodels</h1>
  <h4><p align="center">🚀 Pure Go wrapper for Apple's Foundation Models
</p></h4>
  <p align="center">
    <a href="https://github.com/blacktop/go-foundationmodels/actions" alt="Actions">
          <img src="https://github.com/blacktop/go-foundationmodels/actions/workflows/go.yml/badge.svg" /></a>
    <a href="https://github.com/blacktop/go-foundationmodels/releases/latest" alt="Downloads">
          <img src="https://img.shields.io/github/downloads/blacktop/go-foundationmodels/total.svg" /></a>
    <a href="https://github.com/blacktop/go-foundationmodels/releases" alt="GitHub Release">
          <img src="https://img.shields.io/github/release/blacktop/go-foundationmodels.svg" /></a>
    <a href="http://doge.mit-license.org" alt="LICENSE">
          <img src="https://img.shields.io/:license-mit-blue.svg" /></a>
</p>
<br>

## Why? 🤔

Apple's [Foundation Models](https://developer.apple.com/documentation/foundationmodels) provides powerful on-device AI capabilities in macOS 26 Tahoe, but it's only accessible through Swift/Objective-C APIs. This package bridges that gap, offering:

- **🔒 Privacy-focused**: All AI processing happens on-device, no data leaves your Mac
- **⚡ High performance**: Optimized for Apple Silicon with no network latency
- **🚀 Streaming-first**: Simulated real-time response streaming with typing indicators for modern UX
- **🛠️ Rich tooling**: Advanced features like input validation, context cancellation, and generation control
- **📦 Self-contained**: Embedded Swift shim library - no external dependencies
- **🎯 Production-ready**: Comprehensive error handling, memory management, and structured logging

## Features

### Generation Control
- **Temperature control**: Deterministic (0.0) to creative (1.0) output
- **Token limiting**: Control response length with max tokens
- **Helper functions**: `WithDeterministic()`, `WithCreative()`, `WithBalanced()`

### Advanced Tool System
- **Custom tool creation**: Define tools that Foundation Models can call autonomously
- **Real-time data access**: Via custom integrations
- **Input validation**: Type checking, required fields, enum constraints, regex patterns
- **Automatic error handling**: Comprehensive validation before execution
- **Swift-Go bridge**: Seamless callback mechanism between Foundation Models and Go tools

### Context Management
- **Timeout support**: Cancel long-running requests automatically
- **Manual cancellation**: User-controlled request cancellation
- **Context tracking**: 4096-token window with usage monitoring
- **Session refresh**: Seamless context window management

### Robust Architecture
- **Pure Go implementation**: No CGO dependencies, uses purego for Swift bridge
- **Memory safety**: Automatic C string cleanup and proper resource management
- **Error resilience**: Graceful initialization failure handling
- **Self-contained**: Embedded Swift shim library with automatic extraction
- **Structured logging**: Go slog integration with debug logging for both Go and Swift layers

> [!WARNING]
> I've noticed Apple's model is very finicky and is overly cautious and may refuse when answering any questions.

## Requirements

* **macOS 26 Tahoe** (beta) or later
* **Apple Intelligence enabled** on your device
* **Apple Silicon Mac** (M1/M2/M3/M4 series)
* **Go 1.24+** (uses latest Go features)
* **Xcode 15.x or later** (for Swift shim compilation if needed)

## Getting Started

```bash
go get github.com/blacktop/go-foundationmodels
```

### Basic Usage

```go
package main

import (
    "fmt"
    "log"
    fm "github.com/blacktop/go-foundationmodels"
)

func main() {
    // Check availability
    if fm.CheckModelAvailability() != fm.ModelAvailable {
        log.Fatal("Foundation Models not available")
    }

    // Create session
    sess := fm.NewSession()
    defer sess.Release()

    // Generate text
    response := sess.Respond("What is artificial intelligence?", nil)
    fmt.Println(response)

    // Use generation options
    creative := sess.Respond("Write a story", fm.WithCreative())
    fmt.Println(creative)
}
```

### Tool Calling Example

```go
package main

import (
    "fmt"
    "log"
    fm "github.com/blacktop/go-foundationmodels"
)

// Simple calculator tool
type CalculatorTool struct{}

func (c *CalculatorTool) Name() string { return "calculate" }
func (c *CalculatorTool) Description() string {
    return "Calculate mathematical expressions with add, subtract, multiply, or divide operations"
}
func (c *CalculatorTool) GetParameters() []fm.ToolArgument {
    return []fm.ToolArgument{{
        Name: "arguments", Type: "string", Required: true,
        Description: "Mathematical expression with two numbers and one operation",
    }}
}
func (c *CalculatorTool) Execute(args map[string]any) (fm.ToolResult, error) {
    expr := args["arguments"].(string)
    // ... implement expression parsing and calculation
    return fm.ToolResult{Content: "42.00"}, nil
}

func main() {
    sess := fm.NewSessionWithInstructions("You are a helpful calculator assistant.")
    defer sess.Release()

    // Register tool
    calculator := &CalculatorTool{}
    sess.RegisterTool(calculator)

    // AI will autonomously call the tool when needed
    response := sess.RespondWithTools("What is 15 plus 27?")
    fmt.Println(response) // "The result is 42.00"
}
```

## CLI tool `found`

Install with Homebrew:

```bash
brew install blacktop/tap/found
```

Install with Go:

```bash
go install github.com/blacktop/go-foundationmodels/cmd/found@latest
```

Or download from the latest [release](https://github.com/blacktop/go-foundationmodels/releases/latest)

### CLI Usage

Use `found --help` or `found [command] --help` to see all available commands and examples.

**Available commands:**
- `found info` - Display model availability and system information
- `found quest` - Interactive chat with streaming support, system instructions and JSON output
- `found stream` - Real-time streaming text generation with optional tools ✅
- `found tool calc` - Mathematical calculations with real arithmetic ✅
- `found tool weather` - Real-time weather data with geocoding ✅

![demo](vhs.gif)

## Working Examples

### Tool Calling Success Stories ✅

**Weather Tool**: Get real-time weather data
```bash
found tool weather "New York"
# Returns actual weather from OpenMeteo API with temperature, conditions, humidity, etc.
```

**Calculator Tool**: Perform mathematical operations
```bash
found tool calc "add 15 plus 27"
# Returns: The result of "15 + 27" is **42.00**.
```

**Debug Mode**: See comprehensive logging in action
```bash
found tool weather --verbose "Paris"
# Shows both Go debug logs (slog) and Swift logs with detailed execution flow
```

### Foundation Models Behavior

While tool calling is functional, Foundation Models exhibits some variability:
- ✅ **Tool execution works**: When called, tools successfully return real data
- ✅ **Callback mechanism fixed**: Swift ↔ Go communication is reliable
- ⚠️ **Inconsistent invocation**: Foundation Models sometimes refuses to call tools due to safety restrictions
- ✅ **Error handling**: Graceful failures with helpful explanations

## Known Limitations

- **Foundation Models Safety**: Some queries may be blocked by built-in safety guardrails
- **Context Window**: 4096 token limit requires session refresh for long conversations
- **Tool Parameter Mapping**: Complex expressions may not parse correctly into tool parameters
- **Streaming Implementation**: Currently uses simulated streaming (post-processing chunks) as Foundation Models doesn't yet provide native streaming APIs

## Roadmap

- [x] **Fix tool calling reliability** - ✅ **COMPLETED** - Tools now work with real data
- [x] **Swift-Go callback mechanism** - ✅ **COMPLETED** - Reliable bidirectional communication
- [x] **Tool debugging capabilities** - ✅ **COMPLETED** - `--verbose` flag for comprehensive debug logs
- [x] **Direct tool testing** - ✅ **COMPLETED** - `--direct` flag bypasses Foundation Models
- [x] **Streaming responses** - ✅ **COMPLETED** - Simulated streaming with word/sentence chunks (native streaming pending Foundation Models API)
- [x] **Structured logging** - ✅ **COMPLETED** - Go slog integration with consolidated debug logging
- [ ] **Advanced tool schemas** with OpenAPI-style definitions
- [ ] **Multi-modal support** (images, audio) when available
- [ ] **Performance optimizations** for large contexts
- [ ] **Enhanced error handling** with detailed diagnostics
- [ ] **Plugin system** for extensible tool management
- [ ] **Native streaming support** - Upgrade to Foundation Models native streaming API when available
- [ ] **Improve Foundation Models consistency** - Research better prompting strategies

## License

MIT Copyright (c) 2025 **blacktop**
