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
Sleep 1.5s

# Show Project Information (demo-project is already selected)
Type "i"
Sleep 2s
Type "i" # Toggle off
Sleep 500ms

# Navigate to company-assets-prod
Type "j"
Sleep 1s

# Show Bucket Information
Type "i"
Sleep 2s
Type "i" # Toggle off
Sleep 500ms

# Enter the bucket and explore folders.
Type "l"
Sleep 1s

# Cursor is on "config/". Enter it to see JSON.
Type "l"
Sleep 1s
# Stand on settings.json and show preview
Sleep 2s
Screenshot demo/screenshot.png
# Go back
Type "h"
Sleep 500ms

# Move to "css/"
Type "j"
Sleep 500ms
# Enter to see styles.css
Type "l"
Sleep 2s
# Go back
Type "h"
Sleep 500ms

# Move to "folder/"
Type "j"
Sleep 500ms
# Enter it
Type "l"
Sleep 1s

# Stand on script.py and show Metadata
Type "i"
Sleep 2s
Type "i" # Toggle off
Sleep 500ms

# Show preview of script.py
Sleep 1.5s

# Create a new file "config.yaml"
Type "n"
Sleep 500ms
Type "config.yaml"
Enter
Sleep 1.5s

# Edit the new file with vim to add YAML content
Type "e"
Sleep 1s
Type "i"
Type "server:"
Enter
Type "  port: 8080"
Enter
Type "  timeout: 30s"
Enter
Type "database:"
Enter
Type "  user: admin"
Escape
Sleep 1s
Type ":wq"
Enter
Sleep 2s

# Stand on the newly created config.yaml to show syntax highlighting
Sleep 3s

# Go back from folder/
Type "h"
Sleep 500ms

# Move to "lib/"
Type "j"
Sleep 500ms
# Enter it to see app.jar
Type "l"
Sleep 2s
# Go back
Type "h"
Sleep 500ms

# Move to "index.html"
Type "j"
Sleep 500ms
# Stand on it to show HTML preview
Sleep 2s

# Press H to go back to buckets list
Type "H"
Sleep 1s

# Navigate to db-backups-staging bucket
Type "j"
Sleep 500ms
Type "j"
Sleep 500ms

# Enter db-backups-staging bucket
Type "l"
Sleep 1s

# Press / and show filter
Type "/"
Sleep 1s
Type "2024"
Sleep 2s

# Clear filter
Escape
Sleep 1s

# select 2-3 files, download them
Type " "
Sleep 500ms
Type "j"
Sleep 500ms
Type " "
Sleep 500ms
Type "j"
Sleep 500ms
Type " "
Sleep 500ms
Type "d"
Sleep 2s

# Quit the application
Type "q"
Sleep 500ms
