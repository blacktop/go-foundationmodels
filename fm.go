// Package fm provides a pure Go wrapper around macOS Foundation Models framework
// using purego to call a Swift shim library that exports C functions.
//
// Foundation Models (macOS 26 Tahoe) provides on-device LLM capabilities including:
// - Text generation with LanguageModelSession
// - Streaming responses via delegates or async sequences
// - Tool calling with requestToolInvocation:with:
// - Structured outputs with LanguageModelRequestOptions
//
// IMPORTANT: Foundation Models has a strict 4096 token context window limit.
// This package automatically tracks context usage and validates requests to prevent
// exceeding the limit. Use GetContextSize(), IsContextNearLimit(), and RefreshSession()
// to manage long conversations.
//
// This implementation uses a Swift shim (libFMShim.dylib) that exports C functions
// using @_cdecl to bridge Swift async methods to synchronous C calls.
package fm

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

const MAX_CONTEXT_SIZE = 4096 // Foundation Models context limit

var (
	// Swift shim library handle and function pointers
	shimLib                       uintptr
	createSess                    uintptr
	createSessionWithInstructions uintptr
	releaseSession                uintptr
	checkModelAvailability        uintptr
	respondSync                   uintptr
	respondWithStructuredOutput   uintptr
	respondWithTools              uintptr
	respondWithOptions            uintptr
	getModelInfo                  uintptr
	registerTool                  uintptr
	clearTools                    uintptr
	setToolCallback               uintptr

	// System functions for memory management
	libcFree uintptr

	// Global tool registry
	toolRegistry = make(map[string]Tool)

	// Initialization state
	shimInitialized bool
	shimInitError   error
)

// Embed the Swift shim library
//
//go:embed libFMShim.dylib
var embeddedShimLib []byte

func init() {
	// Initialize the shim on first import
	shimInitError = initializeShim()
	if shimInitError == nil {
		shimInitialized = true
	}
}

// initializeShim loads the Swift shim library and sets up all function pointers
func initializeShim() error {
	// Load the Swift shim library
	var err error
	shimPath := findOrExtractShimLibrary()

	shimLib, err = purego.Dlopen(shimPath, purego.RTLD_NOW)
	if err != nil {
		return fmt.Errorf("failed to load libFMShim.dylib from %s: %v", shimPath, err)
	}

	// Load function symbols from the shim
	createSess, err = purego.Dlsym(shimLib, "CreateSession")
	if err != nil {
		return fmt.Errorf("failed to load CreateSession: %v", err)
	}

	createSessionWithInstructions, err = purego.Dlsym(shimLib, "CreateSessionWithInstructions")
	if err != nil {
		return fmt.Errorf("failed to load CreateSessionWithInstructions: %v", err)
	}

	releaseSession, err = purego.Dlsym(shimLib, "ReleaseSession")
	if err != nil {
		return fmt.Errorf("failed to load ReleaseSession: %v", err)
	}

	checkModelAvailability, err = purego.Dlsym(shimLib, "CheckModelAvailability")
	if err != nil {
		return fmt.Errorf("failed to load CheckModelAvailability: %v", err)
	}

	respondSync, err = purego.Dlsym(shimLib, "RespondSync")
	if err != nil {
		return fmt.Errorf("failed to load RespondSync: %v", err)
	}

	respondWithStructuredOutput, err = purego.Dlsym(shimLib, "RespondWithStructuredOutput")
	if err != nil {
		return fmt.Errorf("failed to load RespondWithStructuredOutput: %v", err)
	}

	respondWithTools, err = purego.Dlsym(shimLib, "RespondWithTools")
	if err != nil {
		return fmt.Errorf("failed to load RespondWithTools: %v", err)
	}

	respondWithOptions, err = purego.Dlsym(shimLib, "RespondWithOptions")
	if err != nil {
		return fmt.Errorf("failed to load RespondWithOptions: %v", err)
	}

	getModelInfo, err = purego.Dlsym(shimLib, "GetModelInfo")
	if err != nil {
		return fmt.Errorf("failed to load GetModelInfo: %v", err)
	}

	registerTool, err = purego.Dlsym(shimLib, "RegisterTool")
	if err != nil {
		return fmt.Errorf("failed to load RegisterTool: %v", err)
	}

	clearTools, err = purego.Dlsym(shimLib, "ClearTools")
	if err != nil {
		return fmt.Errorf("failed to load ClearTools: %v", err)
	}

	setToolCallback, err = purego.Dlsym(shimLib, "SetToolCallback")
	if err != nil {
		return fmt.Errorf("failed to load SetToolCallback: %v", err)
	}

	// Load system libc for memory management
	libcHandle, err := purego.Dlopen("/usr/lib/libc.dylib", purego.RTLD_NOW)
	if err != nil {
		return fmt.Errorf("failed to load libc: %v", err)
	}

	libcFree, err = purego.Dlsym(libcHandle, "free")
	if err != nil {
		return fmt.Errorf("failed to load free function: %v", err)
	}

	// Set up the tool callback
	setupToolCallback()

	return nil
}

// ModelAvailability represents the availability status of the language model
type ModelAvailability int

const (
	ModelAvailable ModelAvailability = iota
	ModelUnavailableAINotEnabled
	ModelUnavailableNotReady
	ModelUnavailableDeviceNotEligible
	ModelUnavailableUnknown = -1
)

// Tool represents a tool that can be called by the Foundation Models
type Tool interface {
	// Name returns the name of the tool
	Name() string
	// Description returns a description of what the tool does
	Description() string
	// Execute executes the tool with the given arguments and returns the result
	Execute(arguments map[string]any) (ToolResult, error)
}

// ValidatedTool extends Tool with input validation capabilities
type ValidatedTool interface {
	Tool
	// ValidateArguments validates the tool arguments before execution
	ValidateArguments(arguments map[string]any) error
}

// ToolArgument represents a tool argument definition for validation
type ToolArgument struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // "string", "number", "integer", "boolean", "array", "object"
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	MinLength   *int     `json:"minLength,omitempty"` // For strings
	MaxLength   *int     `json:"maxLength,omitempty"` // For strings
	Minimum     *float64 `json:"minimum,omitempty"`   // For numbers
	Maximum     *float64 `json:"maximum,omitempty"`   // For numbers
	Pattern     *string  `json:"pattern,omitempty"`   // Regex pattern for strings
	Enum        []any    `json:"enum,omitempty"`      // Allowed values
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

// GenerationOptions represents options for controlling text generation
type GenerationOptions struct {
	// MaxTokens is the maximum number of tokens to generate (default: no limit)
	MaxTokens *int `json:"maxTokens,omitempty"`

	// Temperature controls randomness (0.0 = deterministic, 1.0 = very random)
	Temperature *float32 `json:"temperature,omitempty"`

	// TopP controls nucleus sampling probability threshold (0.0-1.0)
	TopP *float32 `json:"topP,omitempty"`

	// TopK controls top-K sampling limit (positive integer)
	TopK *int `json:"topK,omitempty"`

	// PresencePenalty penalizes tokens based on their presence in the text so far
	PresencePenalty *float32 `json:"presencePenalty,omitempty"`

	// FrequencyPenalty penalizes tokens based on their frequency in the text so far
	FrequencyPenalty *float32 `json:"frequencyPenalty,omitempty"`

	// StopSequences is an array of sequences that will stop generation
	StopSequences []string `json:"stopSequences,omitempty"`

	// Seed for reproducible generation (when temperature is 0.0)
	Seed *int `json:"seed,omitempty"`
}

// Helper functions for creating GenerationOptions

// WithTemperature creates GenerationOptions with specified temperature
func WithTemperature(temp float32) *GenerationOptions {
	return &GenerationOptions{
		Temperature: &temp,
	}
}

// WithMaxTokens creates GenerationOptions with specified max tokens
func WithMaxTokens(maxTokens int) *GenerationOptions {
	return &GenerationOptions{
		MaxTokens: &maxTokens,
	}
}

// WithDeterministic creates GenerationOptions for deterministic output
func WithDeterministic() *GenerationOptions {
	temp := float32(0.0)
	return &GenerationOptions{
		Temperature: &temp,
	}
}

// WithCreative creates GenerationOptions for creative output
func WithCreative() *GenerationOptions {
	temp := float32(0.9)
	return &GenerationOptions{
		Temperature: &temp,
	}
}

// WithBalanced creates GenerationOptions for balanced creativity
func WithBalanced() *GenerationOptions {
	temp := float32(0.7)
	return &GenerationOptions{
		Temperature: &temp,
	}
}

// ToolDefinition represents a tool definition for the Swift shim
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Session represents a LanguageModelSession with context tracking
type Session struct {
	ptr                unsafe.Pointer
	contextSize        int             // Approximate token count
	maxContextSize     int             // Maximum allowed tokens
	systemInstructions string          // System instructions provided at creation
	registeredTools    map[string]Tool // Tools registered with this session
}

// NewSession creates a new LanguageModelSession using the Swift shim
func NewSession() *Session {
	if !shimInitialized {
		fmt.Printf("Foundation Models shim not initialized: %v\n", shimInitError)
		return nil
	}

	ptr, _, _ := purego.SyscallN(createSess)
	if ptr == 0 {
		fmt.Println("Failed to create LanguageModelSession")
		return nil
	}
	return &Session{
		ptr:             unsafe.Pointer(ptr),
		contextSize:     0,
		maxContextSize:  MAX_CONTEXT_SIZE,
		registeredTools: make(map[string]Tool),
	}
}

// NewSessionWithInstructions creates a new LanguageModelSession with system instructions
func NewSessionWithInstructions(instructions string) *Session {
	if !shimInitialized {
		fmt.Printf("Foundation Models shim not initialized: %v\n", shimInitError)
		return nil
	}

	// Validate instructions length
	instructionTokens := estimateTokens(instructions)
	if instructionTokens > 1000 { // Reserve space for conversation
		fmt.Printf("Warning: System instructions are very long (%d tokens). Consider shortening them.\n", instructionTokens)
	}

	cInstructions := cString(instructions)
	ptr, _, _ := purego.SyscallN(createSessionWithInstructions, uintptr(cInstructions))
	if ptr == 0 {
		fmt.Println("Failed to create LanguageModelSession with instructions")
		return nil
	}
	return &Session{
		ptr:                unsafe.Pointer(ptr),
		contextSize:        instructionTokens,
		maxContextSize:     MAX_CONTEXT_SIZE,
		systemInstructions: instructions,
		registeredTools:    make(map[string]Tool),
	}
}

// Release releases the session memory
func (s *Session) Release() {
	if s.ptr != nil {
		purego.SyscallN(releaseSession, uintptr(s.ptr))
		s.ptr = nil
	}
}

// CheckModelAvailability checks if the Foundation Models are available on this device
func CheckModelAvailability() ModelAvailability {
	if !shimInitialized {
		fmt.Printf("Foundation Models shim not initialized: %v\n", shimInitError)
		return ModelUnavailableUnknown
	}

	result, _, _ := purego.SyscallN(checkModelAvailability)
	return ModelAvailability(result)
}

// GetModelInfo returns information about the current language model
func GetModelInfo() string {
	if !shimInitialized {
		return fmt.Sprintf("Foundation Models shim not initialized: %v", shimInitError)
	}

	respPtr, _, _ := purego.SyscallN(getModelInfo)
	if respPtr == 0 {
		return "Error: Could not get model info"
	}

	response := goString(unsafe.Pointer(respPtr))
	freePtr(unsafe.Pointer(respPtr))
	return response
}

// estimateTokens provides a rough estimate of token count for text
// This is a simple approximation: ~4 characters per token on average
func estimateTokens(text string) int {
	// Rough approximation: average of 4 characters per token
	return len(text) / 4
}

// GetContextSize returns the current estimated context size
func (s *Session) GetContextSize() int {
	return s.contextSize
}

// GetMaxContextSize returns the maximum allowed context size
func (s *Session) GetMaxContextSize() int {
	return s.maxContextSize
}

// GetSystemInstructions returns the system instructions for this session
func (s *Session) GetSystemInstructions() string {
	return s.systemInstructions
}

// validateContextSize checks if adding new text would exceed context limit
func (s *Session) validateContextSize(newText string) error {
	newTokens := estimateTokens(newText)
	if s.contextSize+newTokens > s.maxContextSize {
		return fmt.Errorf("context size would exceed limit: current=%d, new=%d, max=%d",
			s.contextSize, newTokens, s.maxContextSize)
	}
	return nil
}

// addToContext adds tokens to the context size tracker
func (s *Session) addToContext(text string) {
	s.contextSize += estimateTokens(text)
}

// GetContextUsagePercent returns the percentage of context used
func (s *Session) GetContextUsagePercent() float64 {
	return float64(s.contextSize) / float64(s.maxContextSize) * 100
}

// IsContextNearLimit returns true if context usage is above 80%
func (s *Session) IsContextNearLimit() bool {
	return s.GetContextUsagePercent() > 80
}

// GetRemainingContextTokens returns the number of tokens remaining in context
func (s *Session) GetRemainingContextTokens() int {
	return s.maxContextSize - s.contextSize
}

// RefreshSession creates a new session with the same system instructions and tools
// This is useful when context is near the limit and you want to continue the conversation
func (s *Session) RefreshSession() *Session {
	var newSess *Session
	if s.systemInstructions != "" {
		newSess = NewSessionWithInstructions(s.systemInstructions)
	} else {
		newSess = NewSession()
	}

	if newSess != nil {
		// Re-register all tools from the old session
		for _, tool := range s.registeredTools {
			newSess.RegisterTool(tool)
		}
	}

	return newSess
}

// RegisterTool registers a tool with the session
func (s *Session) RegisterTool(tool Tool) error {
	if s.ptr == nil {
		return fmt.Errorf("invalid session")
	}

	// Store the tool in the Go registry
	s.registeredTools[tool.Name()] = tool
	toolRegistry[tool.Name()] = tool

	// Create tool definition for Swift shim
	toolDef := ToolDefinition{
		Name:        tool.Name(),
		Description: tool.Description(),
	}

	toolDefJSON, err := json.Marshal(toolDef)
	if err != nil {
		return fmt.Errorf("failed to marshal tool definition: %v", err)
	}

	cToolDef := cString(string(toolDefJSON))

	// Register with Swift shim
	result, _, _ := purego.SyscallN(
		registerTool,
		uintptr(s.ptr),
		uintptr(cToolDef),
	)

	if result == 0 {
		return fmt.Errorf("failed to register tool in Swift shim")
	}

	return nil
}

// ClearTools clears all registered tools from the session
func (s *Session) ClearTools() error {
	if s.ptr == nil {
		return fmt.Errorf("invalid session")
	}

	// Clear from Go registry
	for name := range s.registeredTools {
		delete(toolRegistry, name)
	}
	s.registeredTools = make(map[string]Tool)

	// Clear from Swift shim
	result, _, _ := purego.SyscallN(clearTools, uintptr(s.ptr))
	if result == 0 {
		return fmt.Errorf("failed to clear tools in Swift shim")
	}

	return nil
}

// GetRegisteredTools returns a list of registered tool names
func (s *Session) GetRegisteredTools() []string {
	var tools []string
	for name := range s.registeredTools {
		tools = append(tools, name)
	}
	return tools
}

// toolCallbackFunc is a global variable to keep the callback function alive
var toolCallbackFunc func(cToolName, cArgsJSON unsafe.Pointer) unsafe.Pointer

// setupToolCallback sets up the callback mechanism for Swift to call Go tools
func setupToolCallback() {
	// Create a function pointer that Swift can call
	toolCallbackFunc = func(cToolName, cArgsJSON unsafe.Pointer) unsafe.Pointer {
		toolName := goString(cToolName)
		argsJSON := goString(cArgsJSON)

		result := executeTool(toolName, argsJSON)
		return cString(result)
	}

	// Register the callback with the Swift shim using purego.NewCallback
	callback := purego.NewCallback(toolCallbackFunc)
	purego.SyscallN(setToolCallback, callback)
}

// findOrExtractShimLibrary finds existing shim library or extracts embedded one
func findOrExtractShimLibrary() string {
	// Try to find existing library in various locations
	searchPaths := []string{
		"./libFMShim.dylib",       // Current directory
		"libFMShim.dylib",         // Relative to executable
		"./lib/libFMShim.dylib",   // lib subdirectory
		"./build/libFMShim.dylib", // build subdirectory
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// No existing library found, extract embedded one
	return extractEmbeddedShimLibrary()
}

// extractEmbeddedShimLibrary extracts the embedded shim library to a temporary file
func extractEmbeddedShimLibrary() string {
	// Create a temporary file for the shim library
	tempDir := os.TempDir()
	shimPath := filepath.Join(tempDir, "libFMShim_embedded.dylib")

	// Check if already extracted
	if _, err := os.Stat(shimPath); err == nil {
		fmt.Printf("Using previously extracted shim library at: %s\n", shimPath)
		return shimPath
	}

	// Extract the embedded library
	if err := os.WriteFile(shimPath, embeddedShimLib, 0755); err != nil {
		fmt.Printf("Failed to extract embedded shim library: %v\n", err)
		return ""
	}

	fmt.Printf("Extracted embedded shim library to: %s\n", shimPath)
	return shimPath
}

// executeTool executes a tool by name with the given arguments
// This is called by the Swift shim via a callback
func executeTool(toolName string, argsJSON string) string {
	tool, exists := toolRegistry[toolName]
	if !exists {
		result := ToolResult{
			Error: fmt.Sprintf("tool '%s' not found", toolName),
		}
		resultJSON, _ := json.Marshal(result)
		return string(resultJSON)
	}

	// Parse arguments
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		result := ToolResult{
			Error: fmt.Sprintf("failed to parse arguments: %v", err),
		}
		resultJSON, _ := json.Marshal(result)
		return string(resultJSON)
	}

	// Validate arguments if the tool supports validation
	if validatedTool, ok := tool.(ValidatedTool); ok {
		if err := validatedTool.ValidateArguments(args); err != nil {
			result := ToolResult{
				Error: fmt.Sprintf("validation failed: %v", err),
			}
			resultJSON, _ := json.Marshal(result)
			return string(resultJSON)
		}
	}

	// Execute the tool
	toolResult, err := tool.Execute(args)
	if err != nil {
		toolResult.Error = err.Error()
	}

	// Return result as JSON
	resultJSON, _ := json.Marshal(toolResult)
	return string(resultJSON)
}

// cString creates a null-terminated C string from a Go string
func cString(str string) unsafe.Pointer {
	strBytes := append([]byte(str), 0)
	return unsafe.Pointer(&strBytes[0])
}

// goString converts a C string to a Go string
func goString(cstr unsafe.Pointer) string {
	if cstr == nil {
		return ""
	}

	// Find string length
	length := 0
	for {
		b := *(*byte)(unsafe.Pointer(uintptr(cstr) + uintptr(length)))
		if b == 0 {
			break
		}
		length++
	}

	// Create Go string
	bytes := make([]byte, length)
	for i := 0; i < length; i++ {
		bytes[i] = *(*byte)(unsafe.Pointer(uintptr(cstr) + uintptr(i)))
	}

	return string(bytes)
}

// freePtr safely frees a C pointer using libc's free function
func freePtr(ptr unsafe.Pointer) {
	if ptr != nil && libcFree != 0 {
		purego.SyscallN(libcFree, uintptr(ptr))
	}
}

// Respond sends a prompt to the language model and returns the response
// If options is nil, uses default generation settings
func (s *Session) Respond(prompt string, options *GenerationOptions) string {
	if s.ptr == nil {
		return "Error: Invalid session"
	}

	// Validate context size before sending
	if err := s.validateContextSize(prompt); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	// If options are provided, use RespondWithOptions
	if options != nil {
		// Extract options with defaults
		maxTokens := -1 // -1 means no limit
		if options.MaxTokens != nil {
			maxTokens = *options.MaxTokens
		}

		temperature := float32(0.7) // Default temperature
		if options.Temperature != nil {
			temperature = *options.Temperature
		}

		return s.RespondWithOptions(prompt, maxTokens, temperature)
	}

	cPrompt := cString(prompt)

	// Call RespondSync from the Swift shim
	respPtr, _, _ := purego.SyscallN(
		respondSync,
		uintptr(s.ptr),
		uintptr(cPrompt),
	)

	if respPtr == 0 {
		return "Error: No response from FoundationModels"
	}

	// Convert response to Go string
	response := goString(unsafe.Pointer(respPtr))

	// Free the C string returned by the Swift shim
	freePtr(unsafe.Pointer(respPtr))

	// Update context size with prompt and response
	s.addToContext(prompt)
	s.addToContext(response)

	return response
}

// RespondWithStructuredOutput sends a prompt and returns structured JSON output
func (s *Session) RespondWithStructuredOutput(prompt string) string {
	if s.ptr == nil {
		return "Error: Invalid session"
	}

	// Validate context size before sending
	if err := s.validateContextSize(prompt); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	cPrompt := cString(prompt)

	respPtr, _, _ := purego.SyscallN(
		respondWithStructuredOutput,
		uintptr(s.ptr),
		uintptr(cPrompt),
	)

	if respPtr == 0 {
		return "Error: No response from FoundationModels"
	}

	response := goString(unsafe.Pointer(respPtr))

	// Free the C string returned by the Swift shim
	freePtr(unsafe.Pointer(respPtr))

	// Update context size with prompt and response
	s.addToContext(prompt)
	s.addToContext(response)

	return response
}

// RespondWithTools sends a prompt with tool calling enabled
func (s *Session) RespondWithTools(prompt string) string {
	if s.ptr == nil {
		return "Error: Invalid session"
	}

	// Validate context size before sending
	if err := s.validateContextSize(prompt); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	cPrompt := cString(prompt)

	respPtr, _, _ := purego.SyscallN(
		respondWithTools,
		uintptr(s.ptr),
		uintptr(cPrompt),
	)

	if respPtr == 0 {
		return "Error: No response from FoundationModels"
	}

	response := goString(unsafe.Pointer(respPtr))

	// Free the C string returned by the Swift shim
	freePtr(unsafe.Pointer(respPtr))

	// Update context size with prompt and response
	s.addToContext(prompt)
	s.addToContext(response)

	return response
}

// RespondWithOptions sends a prompt with specific generation options
func (s *Session) RespondWithOptions(prompt string, maxTokens int, temperature float32) string {
	if s.ptr == nil {
		return "Error: Invalid session"
	}

	// Validate context size before sending
	if err := s.validateContextSize(prompt); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	cPrompt := cString(prompt)

	// Convert float32 to uint32 for syscall
	tempUint32 := *(*uint32)(unsafe.Pointer(&temperature))

	respPtr, _, _ := purego.SyscallN(
		respondWithOptions,
		uintptr(s.ptr),
		uintptr(cPrompt),
		uintptr(maxTokens),
		uintptr(tempUint32),
	)

	if respPtr == 0 {
		return "Error: No response from FoundationModels"
	}

	response := goString(unsafe.Pointer(respPtr))

	// Free the C string returned by the Swift shim
	freePtr(unsafe.Pointer(respPtr))

	// Update context size with prompt and response
	s.addToContext(prompt)
	s.addToContext(response)

	return response
}

// Context-aware response methods

// RespondWithContext sends a prompt with context cancellation support
func (s *Session) RespondWithContext(ctx context.Context, prompt string, options *GenerationOptions) (string, error) {
	if s.ptr == nil {
		return "", fmt.Errorf("invalid session")
	}

	// Validate context size before sending
	if err := s.validateContextSize(prompt); err != nil {
		return "", fmt.Errorf("context size validation failed: %v", err)
	}

	// Create a channel to receive the response
	type result struct {
		response string
		err      error
	}
	resultChan := make(chan result, 1)

	// Start the response generation in a goroutine
	go func() {
		var response string
		var err error

		// If options are provided, use RespondWithOptions
		if options != nil {
			// Extract options with defaults
			maxTokens := -1 // -1 means no limit
			if options.MaxTokens != nil {
				maxTokens = *options.MaxTokens
			}

			temperature := float32(0.7) // Default temperature
			if options.Temperature != nil {
				temperature = *options.Temperature
			}

			response = s.RespondWithOptions(prompt, maxTokens, temperature)
		} else {
			response = s.Respond(prompt, nil)
		}

		resultChan <- result{response: response, err: err}
	}()

	// Wait for either completion or context cancellation
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-resultChan:
		if res.err != nil {
			return "", res.err
		}
		return res.response, nil
	}
}

// RespondWithToolsContext sends a prompt with tool calling enabled and context cancellation support
func (s *Session) RespondWithToolsContext(ctx context.Context, prompt string) (string, error) {
	if s.ptr == nil {
		return "", fmt.Errorf("invalid session")
	}

	// Validate context size before sending
	if err := s.validateContextSize(prompt); err != nil {
		return "", fmt.Errorf("context size validation failed: %v", err)
	}

	// Create a channel to receive the response
	type result struct {
		response string
		err      error
	}
	resultChan := make(chan result, 1)

	// Start the response generation in a goroutine
	go func() {
		response := s.RespondWithTools(prompt)
		resultChan <- result{response: response, err: nil}
	}()

	// Wait for either completion or context cancellation
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-resultChan:
		if res.err != nil {
			return "", res.err
		}
		return res.response, nil
	}
}

// RespondWithTimeout is a convenience method that creates a context with timeout
func (s *Session) RespondWithTimeout(timeout time.Duration, prompt string, options *GenerationOptions) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.RespondWithContext(ctx, prompt, options)
}

// RespondWithToolsTimeout is a convenience method for tool calling with timeout
func (s *Session) RespondWithToolsTimeout(timeout time.Duration, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.RespondWithToolsContext(ctx, prompt)
}

// Tool validation helpers

// ValidateToolArguments validates tool arguments against argument definitions
func ValidateToolArguments(args map[string]any, argDefs []ToolArgument) error {
	// Check required arguments
	for _, argDef := range argDefs {
		if argDef.Required {
			if _, exists := args[argDef.Name]; !exists {
				return fmt.Errorf("missing required argument: %s", argDef.Name)
			}
		}
	}

	// Validate each provided argument
	for _, argDef := range argDefs {
		value, exists := args[argDef.Name]
		if !exists {
			continue // Skip optional arguments that weren't provided
		}

		if err := validateArgumentValue(value, argDef); err != nil {
			return fmt.Errorf("invalid argument %s: %v", argDef.Name, err)
		}
	}

	return nil
}

// validateArgumentValue validates a single argument value against its definition
func validateArgumentValue(value any, argDef ToolArgument) error {
	switch argDef.Type {
	case "string":
		return validateStringArgument(value, argDef)
	case "number":
		return validateNumberArgument(value, argDef)
	case "integer":
		return validateIntegerArgument(value, argDef)
	case "boolean":
		return validateBooleanArgument(value, argDef)
	case "array":
		return validateArrayArgument(value, argDef)
	case "object":
		return validateObjectArgument(value, argDef)
	default:
		return fmt.Errorf("unsupported argument type: %s", argDef.Type)
	}
}

// validateStringArgument validates string arguments
func validateStringArgument(value any, argDef ToolArgument) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", value)
	}

	// Check length constraints
	if argDef.MinLength != nil && len(str) < *argDef.MinLength {
		return fmt.Errorf("string too short: %d < %d", len(str), *argDef.MinLength)
	}
	if argDef.MaxLength != nil && len(str) > *argDef.MaxLength {
		return fmt.Errorf("string too long: %d > %d", len(str), *argDef.MaxLength)
	}

	// Check pattern if provided
	if argDef.Pattern != nil {
		matched, err := regexp.MatchString(*argDef.Pattern, str)
		if err != nil {
			return fmt.Errorf("invalid regex pattern: %v", err)
		}
		if !matched {
			return fmt.Errorf("string does not match pattern: %s", *argDef.Pattern)
		}
	}

	// Check enum values if provided
	if len(argDef.Enum) > 0 {
		for _, enumVal := range argDef.Enum {
			if str == enumVal {
				return nil
			}
		}
		return fmt.Errorf("value not in allowed enum values")
	}

	return nil
}

// validateNumberArgument validates number arguments
func validateNumberArgument(value any, argDef ToolArgument) error {
	var num float64

	switch v := value.(type) {
	case float64:
		num = v
	case float32:
		num = float64(v)
	case int:
		num = float64(v)
	case int32:
		num = float64(v)
	case int64:
		num = float64(v)
	default:
		return fmt.Errorf("expected number, got %T", value)
	}

	// Check range constraints
	if argDef.Minimum != nil && num < *argDef.Minimum {
		return fmt.Errorf("number too small: %f < %f", num, *argDef.Minimum)
	}
	if argDef.Maximum != nil && num > *argDef.Maximum {
		return fmt.Errorf("number too large: %f > %f", num, *argDef.Maximum)
	}

	return nil
}

// validateIntegerArgument validates integer arguments
func validateIntegerArgument(value any, argDef ToolArgument) error {
	var num int64

	switch v := value.(type) {
	case int:
		num = int64(v)
	case int32:
		num = int64(v)
	case int64:
		num = v
	case float64:
		// Check if it's actually an integer
		if v != float64(int64(v)) {
			return fmt.Errorf("expected integer, got float with decimal part")
		}
		num = int64(v)
	default:
		return fmt.Errorf("expected integer, got %T", value)
	}

	// Check range constraints
	if argDef.Minimum != nil && float64(num) < *argDef.Minimum {
		return fmt.Errorf("integer too small: %d < %f", num, *argDef.Minimum)
	}
	if argDef.Maximum != nil && float64(num) > *argDef.Maximum {
		return fmt.Errorf("integer too large: %d > %f", num, *argDef.Maximum)
	}

	return nil
}

// validateBooleanArgument validates boolean arguments
func validateBooleanArgument(value any, argDef ToolArgument) error {
	_, ok := value.(bool)
	if !ok {
		return fmt.Errorf("expected boolean, got %T", value)
	}
	return nil
}

// validateArrayArgument validates array arguments
func validateArrayArgument(value any, argDef ToolArgument) error {
	_, ok := value.([]any)
	if !ok {
		return fmt.Errorf("expected array, got %T", value)
	}
	// Could add more specific array validation here
	return nil
}

// validateObjectArgument validates object arguments
func validateObjectArgument(value any, argDef ToolArgument) error {
	_, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("expected object, got %T", value)
	}
	// Could add more specific object validation here
	return nil
}
