package dp

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

//go:embed report_template.html
var reportTemplate string

// RenderHTML 把报告渲染为单一静态 HTML：数据与逻辑全部内嵌，
// 无外部资源、无第三方库、无需启动任何服务，浏览器直接打开即可。
//
// 注入 <script> 的 JSON 必须转义会造成脚本截断或解析歧义的字符：
// 序列 "</script"（即使无闭合尖括号也会被 HTML 解析器当作脚本结束）、
// "<!--"、"<script"，以及在旧 JS 解析器中有特殊含义的行分隔符
// U+2028/U+2029。这里把 <、>、& 与两个行分隔符替换为  转义，
// 它们在 JSON 字符串内合法且语义不变。
var scriptEscaper = strings.NewReplacer(
	"<", "\\u003c",
	">", "\\u003e",
	"&", "\\u0026",
	string(rune(0x2028)), "\\u2028", // LINE SEPARATOR
	string(rune(0x2029)), "\\u2029", // PARAGRAPH SEPARATOR
)

// RenderHTML 把报告渲染为单一静态 HTML 字节流。
func RenderHTML(rep *Report) ([]byte, error) {
	data, err := json.Marshal(rep)
	if err != nil {
		return nil, fmt.Errorf("dp: marshal report: %w", err)
	}
	safe := scriptEscaper.Replace(string(data))
	html := strings.Replace(reportTemplate, "__REPORT_DATA__", safe, 1)
	return []byte(html), nil
}

// WriteHTML 是 RenderHTML 的落盘便捷封装。
func WriteHTML(path string, rep *Report) error {
	out, err := RenderHTML(rep)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}
