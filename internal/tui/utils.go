package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func parentPrefix(p string) string {
	p = strings.TrimSuffix(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i+1]
	}
	return ""
}

func autoRename(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	for i := 1; ; i++ {
		newPath := filepath.Join(dir, fmt.Sprintf("%s_%d%s", name, i, ext))
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
	}
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
	r := []rune(s)
	if len(r) > maxLen {
		if maxLen > 3 {
			return string(r[:maxLen-3]) + "..."
		}
		return string(r[:maxLen])
	}
	return s
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

func getIcon(name string, isFolder bool, isBucket bool) string {
	if isBucket {
		return "🪣 " // Bucket icon
	}
	if isFolder {
		return " " // Folder icon
	}

	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".go":
		return "󰟓 "
	case ".md":
		return " "
	case ".json":
		return " "
	case ".txt":
		return " "
	case ".csv":
		return "󰈙 "
	case ".yaml", ".yml", ".toml":
		return " "
	case ".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp":
		return " "
	case ".pdf":
		return " "
	case ".zip", ".tar", ".gz", ".tgz":
		return " "
	case ".sh", ".bash", ".zsh":
		return " "
	case ".py":
		return " "
	case ".js", ".ts", ".jsx", ".tsx":
		return " "
	case ".html", ".htm":
		return " "
	case ".css":
		return " "
	default:
		return " " // Default file icon
	}
}