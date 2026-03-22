package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	reflowTruncate "github.com/muesli/reflow/truncate"
)

func parentPrefix(p string) string {
	p = strings.TrimSuffix(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i+1]
	}
	return ""
}

func autoRename(path string, activeDestinations map[string]bool) (string, error) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	for i := 1; i <= 100; i++ {
		newPath := filepath.Join(dir, fmt.Sprintf("%s_%d%s", name, i, ext))
		if activeDestinations != nil && activeDestinations[newPath] {
			continue
		}
		_, err := os.Stat(newPath)
		if os.IsNotExist(err) {
			return newPath, nil
		}
		if err != nil {
			return "", fmt.Errorf("failed to check path %q: %w", newPath, err)
		}
	}
	return "", fmt.Errorf("failed to find a free name for %q after 100 attempts", base)
}

func isBinary(s string) bool {
	return strings.ContainsRune(s, '\x00')
}

func fuzzyMatch(query, target string) bool {
	if len(query) == 0 {
		return true
	}
	if len(query) > len(target) {
		return false
	}
	queryRunes := []rune(strings.ToLower(query))

	i := 0
	for _, r := range strings.ToLower(target) {
		if r == queryRunes[i] {
			i++
			if i == len(queryRunes) {
				return true
			}
		}
	}
	return false
}

func visibleRange(cursor, totalItems, maxVisible int) (start, end int) {
	if maxVisible <= 0 {
		return 0, 0
	}
	if totalItems <= maxVisible {
		return 0, totalItems
	}

	start = cursor - maxVisible/2
	if start < 0 {
		start = 0
	}
	end = start + maxVisible
	if end > totalItems {
		end = totalItems
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	return reflowTruncate.StringWithTail(s, uint(maxLen), "...")
}

func humanizeSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

var (
	fallbackIcons = map[string]string{
		".go":   "🐹 ",
		".md":   "📝 ",
		".json": "📄 ",
		".txt":  "📄 ",
		".csv":  "📊 ",
		".yaml": "🛠️ ",
		".yml":  "🛠️ ",
		".toml": "🛠️ ",
		".jpg":  "🖼 ",
		".jpeg": "🖼 ",
		".png":  "🖼 ",
		".gif":  "🖼 ",
		".svg":  "🖼 ",
		".webp": "🖼 ",
		".pdf":  "📕 ",
		".zip":  "📦 ",
		".tar":  "📦 ",
		".gz":   "📦 ",
		".tgz":  "📦 ",
		".sh":   "💻 ",
		".bash": "💻 ",
		".zsh":  "💻 ",
		".py":   "🐍 ",
		".js":   "📜 ",
		".ts":   "📜 ",
		".jsx":  "📜 ",
		".tsx":  "📜 ",
		".html": "🌐 ",
		".htm":  "🌐 ",
		".css":  "🎨 ",
		".xml":  "📋 ",
		".java": "☕ ",
		".c":    "⚙️ ",
		".cpp":  "⚙️ ",
		".h":    "⚙️ ",
		".hpp":  "⚙️ ",
		".rs":   "🦀 ",
		".sql":  "💾 ",
	}

	nerdIcons = map[string]string{
		".go":   "󰟓 ",
		".md":   " ",
		".json": " ",
		".txt":  "󰈙 ",
		".csv":  "󰈛 ",
		".yaml": "󰒓 ",
		".yml":  "󰒓 ",
		".toml": "󰒓 ",
		".jpg":  "󰋩 ",
		".jpeg": "󰋩 ",
		".png":  "󰋩 ",
		".gif":  "󰋩 ",
		".svg":  "󰋩 ",
		".webp": "󰋩 ",
		".pdf":  "󰈦 ",
		".zip":  "󰏗 ",
		".tar":  "󰏗 ",
		".gz":   "󰏗 ",
		".tgz":  "󰏗 ",
		".sh":   " ",
		".bash": " ",
		".zsh":  " ",
		".py":   " ",
		".js":   "󰌞 ",
		".ts":   "󰌞 ",
		".jsx":  "󰌞 ",
		".tsx":  "󰌞 ",
		".html": " ",
		".htm":  " ",
		".css":  "󰌜 ",
		".xml":  "󰗀 ",
		".java": " ",
		".c":    " ",
		".cpp":  " ",
		".h":    " ",
		".hpp":  " ",
		".rs":   " ",
		".sql":  " ",
	}
)

func getIcon(name string, isFolder bool, isBucket bool, useNerdFont bool) string {
	if isBucket {
		if useNerdFont {
			return "🪣 "
		}
		return "📦 "
	}
	if isFolder {
		if useNerdFont {
			return " "
		}
		return "📁 "
	}

	ext := strings.ToLower(filepath.Ext(name))

	if useNerdFont {
		if icon, ok := nerdIcons[ext]; ok {
			return icon
		}
		return "󰈔 " // Default file Nerd Font icon
	}

	if icon, ok := fallbackIcons[ext]; ok {
		return icon
	}
	return "📄 "
}

func getDisplayName(name, currentPrefix string) string {
	return strings.TrimPrefix(name, currentPrefix)
}

func getLevelIcon(level MsgLevel, useNerdFont bool) string {
	if useNerdFont {
		switch level {
		case LevelError:
			return "󰅙 "
		case LevelWarn:
			return "󱈸 "
		case LevelInfo:
			return "󰋽 "
		}
	}
	switch level {
	case LevelError:
		return "❌ "
	case LevelWarn:
		return "⚠️ "
	case LevelInfo:
		return "ℹ️ "
	}
	return ""
}

func renderProgressBar(width int, progress int) string {
	if width < 5 {
		return ""
	}
	filledWidth := int(float64(width) * float64(progress) / 100)
	emptyWidth := width - filledWidth

	filled := strings.Repeat("=", filledWidth)
	if filledWidth > 0 && filledWidth < width {
		filled = filled[:len(filled)-1] + ">"
	}
	empty := strings.Repeat(" ", emptyWidth)

	return "[" + filled + empty + "]"
}
