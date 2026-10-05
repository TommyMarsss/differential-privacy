package dp

import (
	"bytes"
	"math/rand/v2"
	"strings"
	"testing"
	"time"
)

// TestWriteReport 验证报告渲染：包含真实值、加噪结果、剩余预算与被拒绝查询的高亮。
func TestWriteReport(t *testing.T) {
	acct, _ := NewAccountant(1.0)
	e := NewEngine(testDataset(), acct, rand.New(rand.NewPCG(6, 6)))
	results := e.Run([]Query{
		{ID: "Q1", Description: "工程部人数", Epsilon: 0.8, Key: "count:dept=eng",
			Predicate: func(r Record) bool { return r["dept"] == "eng" }},
		{ID: "Q2", Description: "销售部人数", Epsilon: 0.5, Key: "count:dept=sales",
			Predicate: func(r Record) bool { return r["dept"] == "sales" }},
	})
	if !results[1].Rejected {
		t.Fatal("前置条件：Q2 应被预算拒绝")
	}

	var buf bytes.Buffer
	err := WriteReport(&buf, ReportData{
		Title:     "测试报告",
		Dataset:   "test",
		RecordN:   5,
		Generated: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		Summary:   Summarize(results, acct),
		Results:   results,
		Findings:  AnalyzeComposition([]Query{results[0].Query, results[1].Query}),
	})
	if err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{
		"测试报告", "工程部人数", "销售部人数",
		"已拒绝", "已执行", `class="rejected"`,
		"剩余", "真实值", "加噪结果",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("报告缺少内容 %q", want)
		}
	}
	// 不得引用任何外部资源（单一静态文件）。
	for _, bad := range []string{"http://", "https://", "<link ", "src=\"//"} {
		if strings.Contains(html, bad) {
			t.Errorf("报告不应包含外部引用 %q", bad)
		}
	}
}
