package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

// ChatUI provides utilities for creating iPhone vs Android style chat bubbles
type ChatUI struct {
	terminalWidth  int
	maxBubbleWidth int
	blueColor      *color.Color // iPhone iMessage blue
	greenColor     *color.Color // Android green
	spinner        *spinner.Spinner
}

// NewChatUI creates a new chat UI instance
func NewChatUI() *ChatUI {
	return &ChatUI{
		terminalWidth:  80, // Default terminal width
		maxBubbleWidth: 50,
		blueColor:      color.New(color.FgHiBlue, color.Bold),
		greenColor:     color.New(color.FgHiGreen, color.Bold),
	}
}

// PrintUserMessage prints a blue bubble on the right side (iPhone style)
func (c *ChatUI) PrintUserMessage(message string) {
	rightPadding := c.terminalWidth - c.maxBubbleWidth - 2
	questionLines := c.wrapText(message, c.maxBubbleWidth-6) // Space for padding
	bubbleWidth := c.maxTextWidth(questionLines) + 4         // Standard padding
	emojiPad := strings.Repeat(" ", rightPadding-3)          // Position for emoji

	fmt.Println()
	// Blue bubble for user (iPhone iMessage style)
	c.blueColor.Printf("%s🧑 ╭%s╮\n", emojiPad, strings.Repeat("─", bubbleWidth-2))
	for _, line := range questionLines {
		c.blueColor.Printf("%s   │ %-*s │\n", emojiPad, bubbleWidth-4, line)
	}
	c.blueColor.Printf("%s   ╰%s╯\n", emojiPad, strings.Repeat("─", bubbleWidth-2))
}

// PrintAssistantMessage prints a green bubble on the left side (Android style)
func (c *ChatUI) PrintAssistantMessage(message string) {
	responseLines := c.wrapText(message, c.maxBubbleWidth-6) // Space for padding
	responseBubbleWidth := c.maxTextWidth(responseLines) + 4 // Standard padding

	fmt.Println()
	// Green bubble for assistant (Android style)
	c.greenColor.Printf("🤖 ╭%s╮\n", strings.Repeat("─", responseBubbleWidth-2))
	for _, line := range responseLines {
		c.greenColor.Printf("   │ %-*s │\n", responseBubbleWidth-4, line)
	}
	c.greenColor.Printf("   ╰%s╯\n", strings.Repeat("─", responseBubbleWidth-2))
}

// ShowTypingIndicator shows a typing indicator with "found is typing..." message
func (c *ChatUI) ShowTypingIndicator() {
	// Create spinner with dots animation for typing effect
	c.spinner = spinner.New(spinner.CharSets[9], 150*time.Millisecond) // dots: ⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏
	c.spinner.Color("green", "bold")
	c.spinner.Suffix = " found is typing..."
	c.spinner.FinalMSG = ""
	c.spinner.Start()
}

// HideTypingIndicator stops and clears the typing indicator
func (c *ChatUI) HideTypingIndicator() {
	if c.spinner != nil {
		c.spinner.Stop()
		// Clear the line where the spinner was
		fmt.Print("\r\033[K")
	}
}

// PrintContextUsage prints context usage information
func (c *ChatUI) PrintContextUsage(current, max int, percent float64) {
	fmt.Printf("\nContext Usage: %d/%d tokens (%.1f%% used)\n", current, max, percent)
}

// wrapText wraps text to fit within the specified width
func (c *ChatUI) wrapText(text string, width int) []string {
	if len(text) <= width {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	currentLine := ""

	for _, word := range words {
		if currentLine == "" {
			currentLine = word
		} else if len(currentLine+" "+word) <= width {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}

// maxTextWidth returns the maximum width of the given lines
func (c *ChatUI) maxTextWidth(lines []string) int {
	max := 0
	for _, line := range lines {
		if len(line) > max {
			max = len(line)
		}
	}
	return max
}
