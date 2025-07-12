package cmd

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	fm "github.com/blacktop/go-foundationmodels"
	"github.com/spf13/cobra"
)

// CalculatorTool implements basic arithmetic operations
type CalculatorTool struct{}

// Define argument definitions for validation
var calculatorArgDefs = []fm.ToolArgument{
	{
		Name:        "a",
		Type:        "number",
		Description: "First number",
		Required:    true,
	},
	{
		Name:        "b",
		Type:        "number",
		Description: "Second number",
		Required:    true,
	},
	{
		Name:        "operation",
		Type:        "string",
		Description: "Mathematical operation",
		Required:    true,
		Enum:        []any{"add", "subtract", "multiply", "divide", "+", "-", "*", "/"},
	},
}

func (c *CalculatorTool) Name() string {
	return "calculator"
}

func (c *CalculatorTool) Description() string {
	return "Performs mathematical calculations"
}

// ValidateArguments validates the calculator tool arguments
func (c *CalculatorTool) ValidateArguments(args map[string]any) error {
	return fm.ValidateToolArguments(args, calculatorArgDefs)
}

// GetParameters returns the parameter definitions for the calculator tool
func (c *CalculatorTool) GetParameters() []fm.ToolArgument {
	return calculatorArgDefs
}

func (c *CalculatorTool) Execute(args map[string]any) (fm.ToolResult, error) {
	// Extract arguments
	aVal, aExists := args["a"]
	bVal, bExists := args["b"]
	opVal, opExists := args["operation"]
	
	if !aExists || !bExists || !opExists {
		return fm.ToolResult{
			Error: "Missing required arguments: a, b, operation",
		}, nil
	}
	
	// Convert to numbers
	a, err := convertToFloat(aVal)
	if err != nil {
		return fm.ToolResult{
			Error: fmt.Sprintf("Invalid argument 'a': %v", err),
		}, nil
	}
	
	b, err := convertToFloat(bVal)
	if err != nil {
		return fm.ToolResult{
			Error: fmt.Sprintf("Invalid argument 'b': %v", err),
		}, nil
	}
	
	operation, ok := opVal.(string)
	if !ok {
		return fm.ToolResult{
			Error: "Operation must be a string",
		}, nil
	}
	
	// Perform calculation
	var result float64
	switch strings.ToLower(operation) {
	case "add", "+":
		result = a + b
	case "subtract", "-":
		result = a - b
	case "multiply", "*":
		result = a * b
	case "divide", "/":
		if b == 0 {
			return fm.ToolResult{
				Error: "Division by zero",
			}, nil
		}
		result = a / b
	default:
		return fm.ToolResult{
			Error: fmt.Sprintf("Unknown operation: %s (use add, subtract, multiply, divide)", operation),
		}, nil
	}
	
	return fm.ToolResult{
		Content: fmt.Sprintf("%.2f", result),
	}, nil
}

func convertToFloat(val any) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", val)
	}
}

// calcCmd represents the calc command
var calcCmd = &cobra.Command{
	Use:   "calc [mathematical expression or question]",
	Short: "Perform calculations using Foundation Models",
	Long: `Perform mathematical calculations using Foundation Models with a built-in calculator tool.
You can ask natural language questions about math or provide expressions.

⚠️  Note: Tool calling is currently not working reliably with Foundation Models.
This is a beta feature under active development.`,
	Example: `  # Basic arithmetic (Note: may not work reliably)
  found tool calc "What is 15 + 27?"
  found tool calc "Calculate 25% of 200"
  found tool calc "What's 144 divided by 12?"

  # Word problems (Note: may not work reliably)
  found tool calc "If I have 5 apples and buy 3 more, how many do I have?"
  found tool calc "What's the square root of 144?"
  found tool calc "Calculate the area of a circle with radius 5"`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		question := args[0]
		
		// Check model availability
		availability := fm.CheckModelAvailability()
		if availability != fm.ModelAvailable {
			log.Fatalf("Foundation Models not available on this device (status: %d)", availability)
		}
		
		// Create session with calculator instructions
		instructions := `You are a helpful assistant with access to a calculator function.

When users ask math questions:
- ALWAYS use the calculator function for arithmetic
- Never calculate numbers yourself
- Only provide results after using the calculator function
- Show the calculation clearly

You must use the calculator function for all mathematical operations.`
		sess := fm.NewSessionWithInstructions(instructions)
		if sess == nil {
			log.Fatal("Failed to create session")
		}
		defer sess.Release()
		
		// Register calculator tool
		calculator := &CalculatorTool{}
		if err := sess.RegisterTool(calculator); err != nil {
			log.Fatalf("Failed to register calculator tool: %v", err)
		}
		
		fmt.Printf("🧮 Calculator Tool Ready\n")
		fmt.Printf("Question: %s\n", question)
		
		// Get response using tools
		response := sess.RespondWithTools(question)
		
		fmt.Println("\n" + strings.Repeat("=", 50))
		fmt.Println(response)
		fmt.Println(strings.Repeat("=", 50))
		
		// Show context usage
		fmt.Printf("\nContext Usage: %d/%d tokens (%.1f%% used)\n", 
			sess.GetContextSize(), sess.GetMaxContextSize(), sess.GetContextUsagePercent())
	},
}

func init() {
	toolCmd.AddCommand(calcCmd)
}