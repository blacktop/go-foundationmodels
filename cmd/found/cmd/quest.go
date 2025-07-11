package cmd

import (
	"fmt"
	"log"
	"strings"

	fm "github.com/blacktop/go-foundationmodels"
	"github.com/spf13/cobra"
)

var (
	systemInstructions string
	jsonOutput         bool
	temperature        float32
)

// questCmd represents the quest command
var questCmd = &cobra.Command{
	Use:   "quest [prompt]",
	Short: "Ask Foundation Models Questions",
	Long: `Chat with Foundation Models using natural language prompts.
Supports system instructions and structured JSON output.`,
	Example: `  # Basic chat
  found quest "Tell me about machine learning"
  found quest "What is artificial intelligence?"

  # With system instructions
  found quest --system "You are a helpful coding assistant" "Explain Go interfaces"
  found quest --system "You are a concise assistant" "What is Docker?"

  # Structured JSON output
  found quest --json "Analyze this text: 'Hello world'"
  found quest --json "Summarize the key points of machine learning"

  # Control creativity with temperature
  found quest --temp 0.0 "What is 2+2?" # Deterministic
  found quest --temp 0.7 "Tell me about AI" # Balanced
  found quest --temp 1.0 "Write a creative story" # Very creative`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		prompt := args[0]

		// Check model availability
		availability := fm.CheckModelAvailability()
		if availability != fm.ModelAvailable {
			log.Fatalf("Foundation Models not available on this device (status: %d)", availability)
		}

		// Create session with or without system instructions
		var sess *fm.Session
		if systemInstructions != "" {
			fmt.Printf("System Instructions: %s\n", systemInstructions)
			sess = fm.NewSessionWithInstructions(systemInstructions)
		} else {
			sess = fm.NewSession()
		}

		if sess == nil {
			log.Fatal("Failed to create session")
		}
		defer sess.Release()

		// Show initial context if using system instructions
		if systemInstructions != "" {
			fmt.Printf("Initial Context: %d/%d tokens\n", sess.GetContextSize(), sess.GetMaxContextSize())
		}

		fmt.Printf("\nPrompt: %s\n", prompt)

		// Prepare generation options
		var options *fm.GenerationOptions
		if temperature > 0 {
			fmt.Printf("Temperature: %.2f\n", temperature)
			options = &fm.GenerationOptions{
				Temperature: &temperature,
			}
		}

		// Generate response
		var response string
		if jsonOutput {
			fmt.Println("Output Format: JSON")
			response = sess.RespondWithStructuredOutput(prompt)
		} else {
			response = sess.Respond(prompt, options)
		}

		// Display response
		fmt.Println("\n" + strings.Repeat("=", 50))
		fmt.Println(response)
		fmt.Println(strings.Repeat("=", 50))

		// Show final context usage
		fmt.Printf("\nContext Usage: %d/%d tokens (%.1f%% used)\n",
			sess.GetContextSize(), sess.GetMaxContextSize(), sess.GetContextUsagePercent())

		if sess.IsContextNearLimit() {
			fmt.Println("⚠️  Context is near the limit - consider shorter prompts")
		}
	},
}

func init() {
	rootCmd.AddCommand(questCmd)

	// Add flags
	questCmd.Flags().StringVarP(&systemInstructions, "system", "s", "", "System instructions for the model")
	questCmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output structured JSON response")
	questCmd.Flags().Float32VarP(&temperature, "temp", "t", 0, "Temperature for generation (0.0=deterministic, 1.0=creative)")
}
