package cmd

import (
	"fmt"

	fm "github.com/blacktop/go-foundationmodels"
	"github.com/spf13/cobra"
)

// infoCmd represents the info command
var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Display Foundation Models availability and information",
	Long: `Display information about Foundation Models availability on this device,
including model status, capabilities, and system requirements.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("=== Foundation Models Information ===")

		// Check model availability
		availability := fm.CheckModelAvailability()
		fmt.Printf("Model Availability: ")

		switch availability {
		case fm.ModelAvailable:
			fmt.Println("✅ Available")
		case fm.ModelUnavailableAINotEnabled:
			fmt.Println("❌ Apple Intelligence not enabled")
		case fm.ModelUnavailableNotReady:
			fmt.Println("⏳ Model not ready")
		case fm.ModelUnavailableDeviceNotEligible:
			fmt.Println("❌ Device not eligible")
		default:
			fmt.Printf("❓ Unknown status (%d)\n", availability)
		}

		// Get detailed model info
		fmt.Println("\n=== Model Details ===")
		info := fm.GetModelInfo()
		fmt.Print(info)

		// System requirements
		fmt.Println("\n=== System Requirements ===")
		fmt.Println("• macOS 26 Tahoe or later")
		fmt.Println("• Apple Intelligence enabled")
		fmt.Println("• Compatible Apple Silicon device")
		fmt.Println("• Context window: 4096 tokens")

		if availability != fm.ModelAvailable {
			fmt.Println("\n⚠️  Foundation Models is not available on this device.")
			fmt.Println("Please check your macOS version and Apple Intelligence settings.")
		}
	},
}

func init() {
	rootCmd.AddCommand(infoCmd)
}
