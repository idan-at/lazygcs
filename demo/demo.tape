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

# j/k to navigate between the buckets.
Type "j"
Sleep 1s
Type "j"
Sleep 1s
Type "j"
Sleep 1s
Type "k"
Sleep 500ms
Type "k"
Sleep 1s

# Navigate to the bucket and explore folders.
Type "l"
Sleep 1s

# Cursor is on "config/". Enter it to see JSON.
Type "l"
Sleep 2s
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

# Stand on a py file and show syntax highlight (script.py)
Sleep 2s

# Press j and stand on an sql, show syntax highlighting (schema.sql)
Type "j"
Sleep 2s

# Go back
Type "h"
Sleep 500ms

# Move to "lib/"
Type "j"
Sleep 500ms
# Enter it
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

# Press H to go back to buckets list, next bucket
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
