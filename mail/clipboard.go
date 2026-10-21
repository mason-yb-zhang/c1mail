// 跨应用剪贴板：约定 /storage/c1/clipboard.txt，各应用读写同一文件。
package mail

import (
	"os"
	"strings"
)

// DefaultClipboardPath 设备上的共享剪贴板路径。
const DefaultClipboardPath = "/storage/c1/clipboard.txt"

// ClipboardPath 环境变量 C1CLIPBOARD 可覆盖（PC 测试用）。
func ClipboardPath() string {
	if p := os.Getenv("C1CLIPBOARD"); p != "" {
		return p
	}
	return DefaultClipboardPath
}

// WriteClipboard 写入共享剪贴板（整文件覆盖，UTF-8）。
func WriteClipboard(text string) error {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return os.WriteFile(ClipboardPath(), []byte(text), 0o644)
}

// ReadClipboard 读共享剪贴板；不存在或为空返回 ""。
func ReadClipboard() string {
	data, err := os.ReadFile(ClipboardPath())
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(data), "\n")
}
