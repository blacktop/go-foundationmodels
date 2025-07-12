package cmd

import (
	"fmt"
	"log"
	"strings"
	"time"

	fm "github.com/blacktop/go-foundationmodels"
	"github.com/spf13/cobra"
)

// streamCmd represents the stream command
var streamCmd = &cobra.Command{
	Use:   "stream [prompt]",
	Short: "Generate streaming text responses using Foundation Models",
	Long: `Generate streaming text responses using Foundation Models with real-time output.
This demonstrates the streaming capabilities of the Foundation Models framework,
where responses are delivered in chunks as they are generated.

The streaming output provides a more interactive experience compared to waiting
for the complete response.`,
	Example: `  # Basic streaming response
  found stream "Write a short story about a robot"

  # Creative streaming with system instructions
  found stream --instructions "You are a poet" "Write a haiku about mountains"

  # Stream with tools (calculator and weather)
  found stream --tools "What's the weather in Tokyo and calculate 25 * 8?"`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		prompt := args[0]

		// Setup slog based on verbose flag
		verbose, _ := cmd.Flags().GetBool("verbose")
		SetupSlog(verbose)

		// Check model availability
		availability := fm.CheckModelAvailability()
		if availability != fm.ModelAvailable {
			log.Fatalf("Foundation Models not available on this device (status: %d)", availability)
		}

		// Get flags
		instructions, _ := cmd.Flags().GetString("instructions")
		useTools, _ := cmd.Flags().GetBool("tools")

		// Create session
		var sess *fm.Session
		if instructions != "" {
			sess = fm.NewSessionWithInstructions(instructions)
		} else {
			sess = fm.NewSession()
		}

		if sess == nil {
			log.Fatal("Failed to create session")
		}
		defer sess.Release()

		// Register tools if requested
		if useTools {
			calculator := &CalculatorTool{}
			if err := sess.RegisterTool(calculator); err != nil {
				log.Fatalf("Failed to register calculator tool: %v", err)
			}

			weather := &WeatherTool{}
			if err := sess.RegisterTool(weather); err != nil {
				log.Fatalf("Failed to register weather tool: %v", err)
			}

			fmt.Printf("🔧 Registered tools: calculator, weather\n")
		}

		// Create chat UI
		chatUI := NewChatUI()

		// Display user question
		chatUI.PrintUserMessage(prompt)

		fmt.Printf("🚀 Streaming Response\n")

		// Track response for context calculation
		var fullResponse strings.Builder

		// Show typing indicator briefly before streaming starts
		chatUI.ShowTypingIndicator()

		// Create streaming callback
		callback := func(chunk string, isLast bool) {
			// Hide typing indicator on first chunk
			if fullResponse.Len() == 0 && chunk != "" {
				chatUI.HideTypingIndicator()
			}

			if chunk != "" {
				fmt.Print(chunk)
				fullResponse.WriteString(chunk)
			}
			if isLast {
				fmt.Println() // Final newline
			}
		}

		// Start timing
		startTime := time.Now()

		// Choose streaming method based on tools
		if useTools {
			sess.RespondWithToolsStreaming(prompt, callback)
		} else {
			sess.RespondWithStreaming(prompt, callback)
		}

		// Calculate elapsed time
		elapsed := time.Since(startTime)

		fmt.Printf("⏱️  Generated in %v\n", elapsed)

		// Show context usage
		chatUI.PrintContextUsage(sess.GetContextSize(), sess.GetMaxContextSize(), sess.GetContextUsagePercent())

		// Show response stats
		responseLength := fullResponse.Len()
		if responseLength > 0 {
			avgCharsPerSecond := float64(responseLength) / elapsed.Seconds()
			fmt.Printf("📈 Response: %d characters (%.1f chars/sec)\n",
				responseLength, avgCharsPerSecond)
		}

		// Print Swift logs if --verbose flag is set
		if verbose {
			fmt.Println("\n=== Swift Logs ===")
			fmt.Println(fm.GetLogs())
		}

		// Check if context is getting full
		if sess.IsContextNearLimit() {
			fmt.Printf("\n⚠️  Context is near limit. Consider refreshing session for continued use.\n")
		}
	},
}

func init() {
	rootCmd.AddCommand(streamCmd)

	// Add flags
	streamCmd.Flags().StringP("instructions", "i", "", "System instructions for the session")
	streamCmd.Flags().BoolP("tools", "t", false, "Enable calculator and weather tools")
}
