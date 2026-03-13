# Where should we write the GIF?
Output demo/demo.gif

# Set up a 1200x800 terminal with 14px font.
Set FontSize 14
Set Width 1200
Set Height 800
Set Padding 20
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful

# Type a command in the terminal.
Type "lazygcs"
Enter

# Give it some time to load buckets
Sleep 3s

# Move down to the first bucket
Type "j"
Sleep 500ms

# 1. Enter the first bucket (production-assets)
Type "l"
Sleep 1s

# Cursor is now on "config/". Enter it to see the JSON file.
Type "l"
Sleep 2s # Pause to show the syntax-highlighted JSON preview

# Go back to the production-assets directory
Type "h"
Sleep 500ms

# Move down to the "lib/" directory (past css and images)
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 500ms

# Enter "lib/" to see the JAR file preview
Type "l"
Sleep 2s # Pause to show the binary/archive preview of the JAR

# Go back up to the bucket list
Type "h"
Sleep 500ms
Type "h"
Sleep 1s

# 2. Move down to the second bucket (staging-backups)
Type "j"
Sleep 500ms

# Enter the staging-backups bucket
Type "l"
Sleep 1s

# Preview the tar.gz file before downloading
Sleep 2s # Pause to show the tar.gz preview

# 3. Multi-select and download the database backups
# Select the first backup
Type " "
Sleep 500ms

# Move down to the second backup
Type "j"
Sleep 500ms

# Select the second backup
Type " "
Sleep 1s

# Press 'd' to download the selected files
Type "d"
Sleep 3s # Pause to show the downloading status in the UI footer

# 4. Show the Help Menu quickly before quitting
Type "?"
Sleep 2s
Type "?"
Sleep 500ms

# Quit the application
Type "q"
Sleep 1s
