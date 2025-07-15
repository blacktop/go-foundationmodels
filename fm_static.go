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
*/
import "C"
import (
	"fmt"
	"unsafe"
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
	ptr unsafe.Pointer
}

// newCGOSession creates a new session using CGO
func newCGOSession() (SessionInterface, error) {
	if err := checkModelAvailability(); err != nil {
		return nil, err
	}
	ptr := C.CreateSession()
	return &cgoSession{ptr: ptr}, nil
}

// newCGOSessionWithInstructions creates a session with instructions using CGO
func newCGOSessionWithInstructions(instructions string) (SessionInterface, error) {
	if err := checkModelAvailability(); err != nil {
		return nil, err
	}
	cInstructions := C.CString(instructions)
	defer C.free(unsafe.Pointer(cInstructions))
	ptr := C.CreateSessionWithInstructions(cInstructions)
	return &cgoSession{ptr: ptr}, nil
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
	// For now, fall back to basic respond since tool setup is complex
	return s.Respond(prompt)
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
	result, _ := s.cgoSess.RespondWithTools(prompt, nil)
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
	// Store tool and register with session
	// For now, just store it for later use
	return nil
}

func (s *SessionCompat) ClearTools() error {
	// Clear tools from session
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

// GetLogs returns logs from the Swift shim (placeholder)
func GetLogs() string {
	// In CGO version, we don't have the logs functionality yet
	return "Logs not available in CGO version"
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
