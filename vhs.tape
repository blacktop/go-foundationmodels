# VHS documentation
#
# Output:
#   Output <path>.gif               Create a GIF output at the given <path>
#   Output <path>.mp4               Create an MP4 output at the given <path>
#   Output <path>.webm              Create a WebM output at the given <path>
#
# Settings:
#   Set FontSize <Number>           Set the font size of the terminal
#   Set FontFamily <String>         Set the font family of the terminal
#   Set Height <Number>             Set the height of the terminal
#   Set Width <Number>              Set the width of the terminal
#   Set LetterSpacing <Float>       Set the font letter spacing (tracking)
#   Set LineHeight <Float>          Set the font line height
#   Set Theme <String>              Set the theme of the terminal (JSON)
#   Set Padding <Number>[em|px]     Set the padding of the terminal
#   Set Framerate <Number>          Set the framerate of the recording
#   Set PlaybackSpeed <Float>       Set the playback speed of the recording
#
# Sleep:
#   Sleep <time>                    Sleep for a set amount of <time> in seconds
#
# Type:
#   Type[@<time>] "<characters>"    Type <characters> into the terminal with a
#                                   <time> delay between each character
#
# Keys:
#   Backspace[@<time>] [number]     Press the Backspace key
#   Down[@<time>] [number]          Press the Down key
#   Enter[@<time>] [number]         Press the Enter key
#   Space[@<time>] [number]         Press the Space key
#   Tab[@<time>] [number]           Press the Tab key
#   Left[@<time>] [number]          Press the Left Arrow key
#   Right[@<time>] [number]         Press the Right Arrow key
#   Up[@<time>] [number]            Press the Up Arrow key
#   Down[@<time>] [number]          Press the Down Arrow key
#   Ctrl+<key>                      Press the Control key + <key> (e.g. Ctrl+C)
#
# Display:
#   Hide                            Hide the subsequent commands from the output
#   Show                            Show the subsequent commands in the output

Output vhs.gif

Set FontSize 22
Set Width 1200
Set Height 800
Set PlaybackSpeed 2

Type "dist/found tool weather 'San Francisco, California'"

Sleep 500ms

Enter

Sleep 10s

Type "dist/found quest --system 'You are a hopeless romantic' 'Tell me a quick poem about AI'"

Sleep 500ms

Enter

Sleep 10s