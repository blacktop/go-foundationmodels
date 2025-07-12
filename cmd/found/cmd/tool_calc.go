package cmd

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	fm "github.com/blacktop/go-foundationmodels"
	"github.com/spf13/cobra"
)

// CalculatorTool implements basic arithmetic operations
type CalculatorTool struct{}

// Define argument definitions for validation - match Foundation Models' parameter naming
var calculatorArgDefs = []fm.ToolArgument{
	{
		Name:        "arguments",
		Type:        "string",
		Description: "Mathematical expression with two numbers and one operation (add, subtract, multiply, divide)",
		Required:    true,
	},
}

func (c *CalculatorTool) Name() string {
	return "calculate"
}

func (c *CalculatorTool) Description() string {
	return "Calculate mathematical expressions with add, subtract, multiply, or divide operations"
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
	// Extract arguments parameter (matching Foundation Models' naming)
	argsVal, exists := args["arguments"]
	if !exists {
		return fm.ToolResult{
			Error: "Missing required argument: arguments",
		}, nil
	}

	expression, ok := argsVal.(string)
	if !ok {
		return fm.ToolResult{
			Error: "Arguments must be a string",
		}, nil
	}

	// Parse and evaluate the mathematical expression
	result, err := evaluateExpression(expression)
	if err != nil {
		// Check for unsupported operations
		if strings.Contains(err.Error(), "invalid expression format") {
			if containsUnsupportedOperation(expression) {
				return fm.ToolResult{
					Error: "Unsupported operation. Supported operations are: add (+), subtract (-), multiply (*), and divide (/)",
				}, nil
			}
		}
		return fm.ToolResult{
			Error: fmt.Sprintf("Error evaluating expression '%s': %v", expression, err),
		}, nil
	}

	return fm.ToolResult{
		Content: fmt.Sprintf("%.2f", result),
	}, nil
}

// containsUnsupportedOperation checks if the expression contains unsupported operations
func containsUnsupportedOperation(expr string) bool {
	expr = strings.ToLower(expr)
	unsupportedOps := []string{
		"sqrt", "square root", "root", "power", "^", "**",
		"sin", "cos", "tan", "log", "ln", "exp", "abs",
		"mod", "%", "factorial", "!", "pi", "e",
	}

	for _, op := range unsupportedOps {
		if strings.Contains(expr, op) {
			return true
		}
	}
	return false
}

// evaluateExpression parses and evaluates a simple mathematical expression
func evaluateExpression(expr string) (float64, error) {
	// Clean up the expression
	expr = strings.ReplaceAll(expr, " ", "")
	expr = strings.ToLower(expr)

	// Handle common word replacements
	expr = strings.ReplaceAll(expr, "plus", "+")
	expr = strings.ReplaceAll(expr, "add", "+")
	expr = strings.ReplaceAll(expr, "minus", "-")
	expr = strings.ReplaceAll(expr, "subtract", "-")
	expr = strings.ReplaceAll(expr, "times", "*")
	expr = strings.ReplaceAll(expr, "multiply", "*")
	expr = strings.ReplaceAll(expr, "multipliedby", "*")
	expr = strings.ReplaceAll(expr, "dividedby", "/")
	expr = strings.ReplaceAll(expr, "divide", "/")
	expr = strings.ReplaceAll(expr, "×", "*")
	expr = strings.ReplaceAll(expr, "÷", "/")

	// Simple expression parser for basic operations
	// Handle patterns like "5+3", "144/12", "25*8", "100-25"
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*([+\-*/])\s*(\d+(?:\.\d+)?)$`)
	matches := re.FindStringSubmatch(expr)

	if len(matches) != 4 {
		return 0, fmt.Errorf("invalid expression format: %s", expr)
	}

	a, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid first number: %s", matches[1])
	}

	operation := matches[2]

	b, err := strconv.ParseFloat(matches[3], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid second number: %s", matches[3])
	}

	// Perform calculation
	switch operation {
	case "+":
		return a + b, nil
	case "-":
		return a - b, nil
	case "*":
		return a * b, nil
	case "/":
		if b == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return a / b, nil
	default:
		return 0, fmt.Errorf("unknown operation: %s", operation)
	}
}

// convertToFloat is currently unused but kept for future use
//
//nolint:unused
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

		// Setup slog based on verbose flag
		verbose, _ := cmd.Flags().GetBool("verbose")
		SetupSlog(verbose)

		// Check model availability
		availability := fm.CheckModelAvailability()
		if availability != fm.ModelAvailable {
			log.Fatalf("Foundation Models not available on this device (status: %d)", availability)
		}

		// Create session with calculator instructions
		instructions := `You are a helpful assistant with access to a calculate function.

The calculate function supports ONLY these operations:
- Addition (add, plus, +)
- Subtraction (subtract, minus, -)
- Multiplication (multiply, times, *)
- Division (divide, /)

When users ask mathematical questions:
- ALWAYS use the calculate function with basic math expressions
- Convert natural language to mathematical expressions (e.g., "2 plus 2" becomes "2 + 2")
- For unsupported operations (square root, powers, etc.), explain what operations are supported
- Never perform calculations yourself

You must use the calculate function for all supported mathematical operations.`
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

		// Create chat UI
		chatUI := NewChatUI()

		// Display user question
		chatUI.PrintUserMessage(question)

		// Show typing indicator while waiting for response
		chatUI.ShowTypingIndicator()

		// Get response using tools
		response := sess.RespondWithTools(question)

		// Hide typing indicator and display assistant response
		chatUI.HideTypingIndicator()
		chatUI.PrintAssistantMessage(response)

		// Show context usage
		chatUI.PrintContextUsage(sess.GetContextSize(), sess.GetMaxContextSize(), sess.GetContextUsagePercent())

		// Print Swift logs if --verbose flag is set
		if verbose {
			fmt.Println("\n=== Swift Logs ===")
			fmt.Println(fm.GetLogs())
		}
	},
}

func init() {
	toolCmd.AddCommand(calcCmd)
}
