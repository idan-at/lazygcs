package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

func highlightMatch(s, query string, isFuzzy bool) string {
	if query == "" {
		return s
	}

	highlightStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#1E1E2E")).Background(lipgloss.Color("#F9E2AF")).Bold(true)

	if !isFuzzy {
		lowerS := strings.ToLower(s)
		lowerQ := strings.ToLower(query)
		idx := strings.Index(lowerS, lowerQ)
		if idx == -1 {
			return s
		}

		match := s[idx : idx+len(query)]
		return s[:idx] + highlightStyle.Render(match) + s[idx+len(query):]
	}

	// Fuzzy highlight: highlight matching characters
	queryRunes := []rune(strings.ToLower(query))
	targetRunes := []rune(s)
	targetLowerRunes := []rune(strings.ToLower(s))

	var result strings.Builder
	qIdx := 0

	for i, r := range targetRunes {
		if qIdx < len(queryRunes) && targetLowerRunes[i] == queryRunes[qIdx] {
			result.WriteString(highlightStyle.Render(string(r)))
			qIdx++
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
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
	fallbackExactIcons = map[string]string{
		"dockerfile":          "🐳 ",
		"makefile":            "🛠️ ",
		".gitignore":          "👁️ ",
		"docker-compose.yml":  "🐳 ",
		"docker-compose.yaml": "🐳 ",
	}

	nerdExactIcons = map[string]string{
		"dockerfile":          " ",
		"makefile":            " ",
		".gitignore":          " ",
		"docker-compose.yml":  " ",
		"docker-compose.yaml": " ",
	}

	fallbackIcons = map[string]string{
		".go":   "🐹 ",
		".md":   "📝 ",
		".json": "📋 ",
		".txt":  "📄 ",
		".csv":  "📊 ",
		".yaml": "🔧 ",
		".yml":  "🔧 ",
		".toml": "🔧 ",
		".jpg":  "🌅 ",
		".jpeg": "🌅 ",
		".png":  "🌅 ",
		".gif":  "🌅 ",
		".svg":  "🌅 ",
		".webp": "🌅 ",
		".pdf":  "📕 ",
		".zip":  "🧳 ",
		".tar":  "🧳 ",
		".gz":   "🧳 ",
		".tgz":  "🧳 ",
		".sh":   "🐚 ",
		".bash": "🐚 ",
		".zsh":  "🐚 ",
		".py":   "🐍 ",
		".js":   "🟨 ",
		".ts":   "🟦 ",
		".jsx":  "🧬 ",
		".tsx":  "🧬 ",
		".html": "🌐 ",
		".htm":  "🌐 ",
		".css":  "🎨 ",
		".xml":  "📑 ",
		".java": "☕ ",
		".c":    "🧱 ",
		".cpp":  "🧱 ",
		".h":    "🧪 ",
		".hpp":  "🧪 ",
		".rs":   "🦀 ",
		".sql":  "💾 ",
		".env":  "🔐 ",
		".lock": "🔒 ",
		".rb":   "💎 ",
		".php":  "🐘 ",
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
		".zip":  " ",
		".tar":  " ",
		".gz":   " ",
		".tgz":  " ",
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
		".env":  "󰒓 ",
		".lock": "󰌾 ",
		".rb":   " ",
		".php":  "󰌽 ",
	}
)

func getIcon(name string, isFolder bool, isBucket bool, useNerdFont bool) string {
	if isBucket {
		if useNerdFont {
			return " " // Database icon for buckets
		}
		return "📦 "
	}
	if isFolder {
		if useNerdFont {
			return "󰉋 "
		}
		return "📁 "
	}

	baseName := strings.ToLower(filepath.Base(name))
	ext := strings.ToLower(filepath.Ext(name))

	if useNerdFont {
		if icon, ok := nerdExactIcons[baseName]; ok {
			return icon
		}
		if icon, ok := nerdIcons[ext]; ok {
			return icon
		}
		return "󰈔 " // Default file Nerd Font icon
	}

	if icon, ok := fallbackExactIcons[baseName]; ok {
		return icon
	}
	if icon, ok := fallbackIcons[ext]; ok {
		return icon
	}
	return "📄 "
}

func getIconColor(name string, isFolder bool, isBucket bool) string {
	if isBucket {
		return "#CBA6F7" // Mauve
	}
	if isFolder {
		return "#8CAAEE" // Blue
	}

	baseName := strings.ToLower(filepath.Base(name))
	ext := strings.ToLower(filepath.Ext(name))

	// Exact matches
	switch baseName {
	case "dockerfile", "docker-compose.yml", "docker-compose.yaml":
		return "#8CAAEE" // Blue
	case "makefile":
		return "#A6ADC8" // Subtext0
	case ".gitignore":
		return "#F38BA8" // Red
	}

	// Extensions
	switch ext {
	case ".go":
		return "#89DCEB" // Sky
	case ".md", ".txt":
		return "#BAC2DE" // Subtext1
	case ".json", ".yaml", ".yml", ".toml":
		return "#FAB387" // Peach
	case ".csv":
		return "#A6E3A1" // Green
	case ".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp":
		return "#F5C2E7" // Pink
	case ".pdf":
		return "#F38BA8" // Red
	case ".zip", ".tar", ".gz", ".tgz":
		return "#F38BA8" // Red
	case ".sh", ".bash", ".zsh":
		return "#A6E3A1" // Green
	case ".py":
		return "#F9E2AF" // Yellow
	case ".js", ".ts", ".jsx", ".tsx":
		return "#F9E2AF" // Yellow
	case ".html", ".htm", ".css":
		return "#8CAAEE" // Blue
	case ".xml":
		return "#FAB387" // Peach
	case ".java", ".c", ".cpp", ".h", ".hpp", ".rs":
		return "#F9E2AF" // Yellow
	case ".sql":
		return "#FAB387" // Peach
	case ".env", ".lock":
		return "#F38BA8" // Red
	case ".rb", ".php":
		return "#F38BA8" // Red
	}

	return "#A6ADC8" // Default (Subtext0)
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
