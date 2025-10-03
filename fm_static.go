//go:build darwin && arm64 && cgo
// +build darwin,arm64,cgo

package fm

//go:generate bash -c "swiftc -sdk $(xcrun --show-sdk-path) -target arm64-apple-macos26 -emit-object -parse-as-library -whole-module-optimization -O -o libFMShim.o FoundationModelsShim.swift"
//go:generate ar rcs libFMShim.a libFMShim.o
//go:generate rm -f libFMShim.o

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: ${SRCDIR}/libFMShim.a -framework Foundation -framework FoundationModels -lc++ -lobjc

#include <stdlib.h>
#include <string.h>

// Forward declare strdup for C99 compatibility
char *strdup(const char *s);

// Declare the Swift functions we're importing
void* CreateSession(void);
void* CreateSessionWithInstructions(const char* instructions);
void ReleaseSession(void* session);
int CheckModelAvailability(void);
char* RespondSync(void* session, const char* prompt);
char* GetModelInfo(void);
char* GetLogs(void);

// Tool calling functions
int RegisterTool(void* session, const char* toolDefJSON);
int ClearTools(void* session);
char* RespondWithTools(void* session, const char* prompt);
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"
)

// Global tool registry for CGO callbacks
var (
	toolRegistry   = make(map[string]Tool)
	toolRegistryMu sync.RWMutex
)

// toolExecuteCallback is the CGO callback that Swift calls when a tool needs to be executed
//
//export toolExecuteCallback
func toolExecuteCallback(cToolName *C.char, cArgsJSON *C.char) *C.char {
	toolName := C.GoString(cToolName)
	argsJSON := C.GoString(cArgsJSON)

	// Look up the tool
	toolRegistryMu.RLock()
	tool, exists := toolRegistry[toolName]
	toolRegistryMu.RUnlock()

	if !exists {
		result := map[string]string{"error": fmt.Sprintf("Tool not found: %s", toolName)}
		resultJSON, _ := json.Marshal(result)
		return C.CString(string(resultJSON))
	}

	// Parse the outer wrapper that contains {"arguments": "..."}
	var wrapper map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &wrapper); err != nil {
		result := map[string]string{"error": fmt.Sprintf("Failed to parse argument wrapper: %v", err)}
		resultJSON, _ := json.Marshal(result)
		return C.CString(string(resultJSON))
	}

	// Extract the inner arguments JSON string
	argsStr, ok := wrapper["arguments"].(string)
	if !ok {
		result := map[string]string{"error": "Arguments field is not a string"}
		resultJSON, _ := json.Marshal(result)
		return C.CString(string(resultJSON))
	}

	// Parse the actual arguments
	var args map[string]any
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		result := map[string]string{"error": fmt.Sprintf("Failed to parse inner arguments: %v", err)}
		resultJSON, _ := json.Marshal(result)
		return C.CString(string(resultJSON))
	}

	// Execute the tool
	toolResult, err := tool.Execute(args)
	if err != nil {
		result := map[string]string{"error": err.Error()}
		resultJSON, _ := json.Marshal(result)
		return C.CString(string(resultJSON))
	}

	// Return result as JSON
	resultJSON, _ := json.Marshal(toolResult)
	return C.CString(string(resultJSON))
}

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

// SchematizedTool extends Tool with parameter schema definition capabilities
type SchematizedTool interface {
	Tool
	// GetParameters returns the parameter definitions for this tool
	GetParameters() []ToolArgument
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

// Session interface for compatibility
type SessionInterface interface {
	Respond(prompt string) (string, error)
	RespondWithTools(prompt string, tools []Tool) (string, error)
	RespondWithOptions(prompt string, options *GenerationOptions) (string, error)
	RespondStreaming(prompt string, callback func(chunk string, isDone bool)) error
	RespondWithToolsStreaming(prompt string, tools []Tool, callback func(chunk string, isDone bool)) error
	Close()
}

// CGO-based session implementation
type cgoSession struct {
	ptr           unsafe.Pointer
	tools         []Tool
	callbackSet   bool
	callbackSetMu sync.Mutex
}

// newCGOSession creates a new session using CGO
func newCGOSession() (SessionInterface, error) {
	if err := checkModelAvailability(); err != nil {
		return nil, err
	}
	ptr := C.CreateSession()
	sess := &cgoSession{ptr: ptr}
	sess.ensureCallbackSet()
	return sess, nil
}

// newCGOSessionWithInstructions creates a session with instructions using CGO
func newCGOSessionWithInstructions(instructions string) (SessionInterface, error) {
	if err := checkModelAvailability(); err != nil {
		return nil, err
	}
	cInstructions := C.CString(instructions)
	defer C.free(unsafe.Pointer(cInstructions))
	ptr := C.CreateSessionWithInstructions(cInstructions)
	sess := &cgoSession{ptr: ptr}
	sess.ensureCallbackSet()
	return sess, nil
}

// ensureCallbackSet is a no-op now since Swift directly calls the exported Go function
func (s *cgoSession) ensureCallbackSet() {
	// The toolExecuteCallback function is exported via //export directive
	// Swift uses @_silgen_name to call it directly - no setup needed
}

func checkModelAvailability() error {
	status := C.CheckModelAvailability()
	switch status {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("Apple Intelligence not enabled")
	case 2:
		return fmt.Errorf("model not ready")
	case 3:
		return fmt.Errorf("device not eligible")
	default:
		return fmt.Errorf("unknown availability status: %d", status)
	}
}

func (s *cgoSession) Respond(prompt string) (string, error) {
	cPrompt := C.CString(prompt)
	defer C.free(unsafe.Pointer(cPrompt))

	result := C.RespondSync(s.ptr, cPrompt)
	defer C.free(unsafe.Pointer(result))

	return C.GoString(result), nil
}

func (s *cgoSession) RespondWithTools(prompt string, tools []Tool) (string, error) {
	// Register all tools with the session
	for _, tool := range tools {
		if err := s.registerTool(tool); err != nil {
			return "", fmt.Errorf("failed to register tool %s: %w", tool.Name(), err)
		}
	}

	// Call the Swift RespondWithTools function
	cPrompt := C.CString(prompt)
	defer C.free(unsafe.Pointer(cPrompt))

	result := C.RespondWithTools(s.ptr, cPrompt)
	defer C.free(unsafe.Pointer(result))

	return C.GoString(result), nil
}

// registerTool registers a single tool with the session
func (s *cgoSession) registerTool(tool Tool) error {
	// Add to session's tool list
	s.tools = append(s.tools, tool)

	// Add to global registry for callback lookups
	toolRegistryMu.Lock()
	toolRegistry[tool.Name()] = tool
	toolRegistryMu.Unlock()

	// Create tool definition for Swift
	toolDef := struct {
		Name        string                    `json:"name"`
		Description string                    `json:"description"`
		Parameters  map[string]map[string]any `json:"parameters,omitempty"`
	}{
		Name:        tool.Name(),
		Description: tool.Description(),
		Parameters:  make(map[string]map[string]any),
	}

	// Add parameters if tool supports schema
	if schematizedTool, ok := tool.(SchematizedTool); ok {
		for _, arg := range schematizedTool.GetParameters() {
			paramDef := map[string]any{
				"type":        arg.Type,
				"description": arg.Description,
				"required":    arg.Required,
			}
			if len(arg.Enum) > 0 {
				paramDef["enum"] = arg.Enum
			}
			toolDef.Parameters[arg.Name] = paramDef
		}
	}

	// Marshal to JSON
	toolDefJSON, err := json.Marshal(toolDef)
	if err != nil {
		return fmt.Errorf("failed to marshal tool definition: %w", err)
	}

	// Register with Swift
	cToolDef := C.CString(string(toolDefJSON))
	defer C.free(unsafe.Pointer(cToolDef))

	status := C.RegisterTool(s.ptr, cToolDef)
	if status == 0 {
		return fmt.Errorf("failed to register tool with Swift (see logs)")
	}

	return nil
}

func (s *cgoSession) RespondWithOptions(prompt string, options *GenerationOptions) (string, error) {
	// For now, fall back to basic respond
	return s.Respond(prompt)
}

func (s *cgoSession) RespondStreaming(prompt string, callback func(chunk string, isDone bool)) error {
	result, err := s.Respond(prompt)
	if err != nil {
		callback(err.Error(), true)
		return err
	}
	callback(result, true)
	return nil
}

func (s *cgoSession) RespondWithToolsStreaming(prompt string, tools []Tool, callback func(chunk string, isDone bool)) error {
	result, err := s.RespondWithTools(prompt, tools)
	if err != nil {
		callback(err.Error(), true)
		return err
	}
	callback(result, true)
	return nil
}

func (s *cgoSession) Close() {
	if s.ptr != nil {
		C.ReleaseSession(s.ptr)
		s.ptr = nil
	}
}

// GetModelInfo returns information about the Foundation Models system (single return for compatibility)
func GetModelInfo() string {
	result := C.GetModelInfo()
	defer C.free(unsafe.Pointer(result))
	return C.GoString(result)
}

// Compatibility functions for the CLI

// SessionCompat represents a LanguageModelSession (compatibility with purego version)
type SessionCompat struct {
	cgoSess *cgoSession
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

// CheckModelAvailability checks if the Foundation Models are available
func CheckModelAvailability() ModelAvailability {
	status := C.CheckModelAvailability()
	switch status {
	case 0:
		return ModelAvailable
	case 1:
		return ModelUnavailableAINotEnabled
	case 2:
		return ModelUnavailableNotReady
	case 3:
		return ModelUnavailableDeviceNotEligible
	default:
		return ModelUnavailableUnknown
	}
}

// Session represents a LanguageModelSession (matches purego API)
type Session = SessionCompat

// Compatibility wrapper to match purego API
func NewSession() *SessionCompat {
	session, err := newCGOSession()
	if err != nil || session == nil {
		return nil
	}
	return &SessionCompat{cgoSess: session.(*cgoSession)}
}

// Compatibility wrapper to match purego API
func NewSessionWithInstructions(instructions string) *SessionCompat {
	session, err := newCGOSessionWithInstructions(instructions)
	if err != nil || session == nil {
		return nil
	}
	return &SessionCompat{cgoSess: session.(*cgoSession)}
}

// Compatibility methods for Session struct

func (s *SessionCompat) Respond(prompt string, options *GenerationOptions) string {
	result, _ := s.cgoSess.Respond(prompt)
	return result
}

func (s *SessionCompat) RespondWithTools(prompt string) string {
	// Use the tools registered with the session
	result, _ := s.cgoSess.RespondWithTools(prompt, s.cgoSess.tools)
	return result
}

func (s *SessionCompat) RespondWithOptions(prompt string, maxTokens int, temperature float32) string {
	options := &GenerationOptions{
		Temperature: &temperature,
	}
	if maxTokens > 0 {
		options.MaxTokens = &maxTokens
	}
	result, _ := s.cgoSess.RespondWithOptions(prompt, options)
	return result
}

func (s *SessionCompat) RespondStreaming(prompt string, callback func(chunk string, isDone bool)) {
	s.cgoSess.RespondStreaming(prompt, callback)
}

func (s *SessionCompat) RespondWithToolsStreaming(prompt string, callback func(chunk string, isDone bool)) {
	s.cgoSess.RespondWithToolsStreaming(prompt, nil, callback)
}

// Compatibility methods expected by CLI
func (s *SessionCompat) Release() {
	s.cgoSess.Close()
}

func (s *SessionCompat) GetContextSize() int {
	return 0 // Not tracked in CGO version
}

func (s *SessionCompat) GetMaxContextSize() int {
	return 4096 // Foundation Models limit
}

func (s *SessionCompat) RespondWithStructuredOutput(prompt string) string {
	// Not implemented in CGO version yet, fall back to basic respond
	result, _ := s.cgoSess.Respond(prompt)
	return result
}

// Additional methods for tool management
func (s *SessionCompat) RegisterTool(tool Tool) error {
	return s.cgoSess.registerTool(tool)
}

func (s *SessionCompat) ClearTools() error {
	// Clear from Swift session
	status := C.ClearTools(s.cgoSess.ptr)
	if status == 0 {
		return fmt.Errorf("failed to clear tools (see logs)")
	}
	// Clear from Go session
	s.cgoSess.tools = nil
	// Clear from global registry
	toolRegistryMu.Lock()
	toolRegistry = make(map[string]Tool)
	toolRegistryMu.Unlock()

	return nil
}

// Missing methods expected by CLI
func (s *SessionCompat) GetContextUsagePercent() float64 {
	return 0.0 // Not tracked in CGO version
}

func (s *SessionCompat) IsContextNearLimit() bool {
	return false // Not tracked in CGO version
}

func (s *SessionCompat) RespondWithStreaming(prompt string, callback func(chunk string, isDone bool)) {
	s.cgoSess.RespondStreaming(prompt, callback)
}

// GetLogs returns logs from the Swift shim
func GetLogs() string {
	result := C.GetLogs()
	defer C.free(unsafe.Pointer(result))
	return C.GoString(result)
}

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
	// Basic validation - could be expanded
	return nil
}
