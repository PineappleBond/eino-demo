package utils

// TruncateString 截断字符串，如果超出最大长度则返回截断的字符串+"..."
// maxLen 为字符数（不是字节数），适用于中英文混合字符串
func TruncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
