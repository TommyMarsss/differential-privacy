package dp

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
)

func TestRenderHTMLSelfContained(t *testing.T) {
	ds := &Dataset{Name: "d", Records: []Record{{"g": "a"}, {"g": "b"}}}
	engine, _ := NewEngine(1.0, rand.New(rand.NewPCG(7, 7)))
	rep := engine.Run("测试报告", ds, []Query{
		{Name: "q0", Description: "全部", Epsilon: 0.4, Predicate: func(r Record) bool { return true }},
		{Name: "q1", Description: "超额", Epsilon: 0.9, Predicate: func(r Record) bool { return true }},
	})

	html, err := RenderHTML(rep)
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)

	if strings.Contains(s, "__REPORT_DATA__") {
		t.Error("模板占位符必须被替换")
	}
	for _, ext := range []string{"http://", "https://", `src="`, "<link", "cdn."} {
		if strings.Contains(strings.ToLower(s), ext) {
			t.Errorf("报告必须完全自包含，发现外部引用: %s", ext)
		}
	}
	if !strings.Contains(s, "q0") || !strings.Contains(s, "exceeding budget") {
		t.Error("报告应包含查询名与拒绝原因")
	}
}

// TestRenderHTMLEscapesScriptBreaking 验证查询名中的 </script> 等注入串
// 被转义，无法截断脚本；且嵌入的 JSON 仍可被还原解析。
func TestRenderHTMLEscapesScriptBreaking(t *testing.T) {
	rep := &Report{
		Title: "evil",
		Results: []Result{{
			Index:        0,
			Name:         "</script><script>alert(1)</script>",
			Description:  "<!-- <script> x & y   z",
			Accepted:     false,
			RejectReason: "reject",
		}},
		Findings: []Finding{},
	}
	html, err := RenderHTML(rep)
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	if strings.Contains(s, "</script><script>alert") {
		t.Error("原始 </script> 注入串必须被转义")
	}
	if strings.Contains(s, " ") {
		t.Error("U+2028 必须被转义")
	}

	// 抽取 const REPORT = ...; 中的 JSON 文本，反转义后必须仍是合法 JSON，
	// 且数据语义（查询名）完好。
	start := strings.Index(s, "const REPORT = ")
	if start < 0 {
		t.Fatal("找不到 REPORT 数据")
	}
	line := s[start+len("const REPORT = "):]
	end := strings.Index(line, ";")
	rawJSON := line[:end]
	if !strings.Contains(rawJSON, "alert(1)") {
		t.Error("转义后的 JSON 中应仍保留 alert(1) 文本（语义不变）")
	}
	decoded := strings.NewReplacer(
		"\\u003c", "<", "\\u003e", ">", "\\u0026", "&",
		"\\u2028", string(rune(0x2028)),
		"\\u2029", string(rune(0x2029)),
	).Replace(rawJSON)
	var got Report
	if err := json.Unmarshal([]byte(decoded), &got); err != nil {
		t.Fatalf("转义后数据应仍是合法 JSON: %v", err)
	}
	if got.Results[0].Name != "</script><script>alert(1)</script>" {
		t.Errorf("还原后的查询名被破坏: %q", got.Results[0].Name)
	}
}

func TestWriteHTML(t *testing.T) {
	rep := &Report{Title: "x", Results: []Result{}, Findings: []Finding{}}
	path := t.TempDir() + "/report.html"
	if err := WriteHTML(path, rep); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "<!DOCTYPE html>") {
		t.Error("输出应为完整 HTML 文档")
	}
}
