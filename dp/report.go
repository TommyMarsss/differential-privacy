package dp

import (
	"fmt"
	"html/template"
	"io"
	"math"
	"time"
)

// ReportData 是渲染报告所需的全部数据。
type ReportData struct {
	Title     string
	Dataset   string
	RecordN   int
	Generated time.Time
	Summary   Summary
	Results   []Result
	Findings  []Finding
}

// WriteReport 将一次查询运行渲染为单一静态 HTML 文件（不依赖任何外部资源）。
func WriteReport(w io.Writer, data ReportData) error {
	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"f2":  func(v float64) string { return fmt.Sprintf("%.2f", v) },
		"f4":  func(v float64) string { return fmt.Sprintf("%.4f", v) },
		"pct": func(v float64) string { return fmt.Sprintf("%.1f%%", v*100) },
		"div": func(a, b float64) float64 { return a / b },
		"err": func(r Result) string { return fmt.Sprintf("%.2f", math.Abs(r.NoisyCount-float64(r.TrueCount))) },
	}).Parse(reportTemplate)
	if err != nil {
		return fmt.Errorf("dp: 解析报告模板失败: %w", err)
	}
	if err := tmpl.Execute(w, data); err != nil {
		return fmt.Errorf("dp: 渲染报告失败: %w", err)
	}
	return nil
}

const reportTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
  :root {
    --bg: #f6f7f9; --card: #ffffff; --text: #1c2330; --muted: #5b6575;
    --border: #dfe3ea; --accent: #2456d6;
    --ok-bg: #e8f5ec; --ok-fg: #176c33;
    --bad-bg: #fdeceb; --bad-fg: #b42318;
    --warn-bg: #fdf3e0; --warn-fg: #93540a;
    --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      --bg: #14171d; --card: #1d222c; --text: #e7ebf2; --muted: #9aa5b5;
      --border: #2c333f; --accent: #7ea2ff;
      --ok-bg: #14321f; --ok-fg: #7fd49a;
      --bad-bg: #3a1d1b; --bad-fg: #ff9b94;
      --warn-bg: #382a12; --warn-fg: #f3c273;
    }
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; background: var(--bg); color: var(--text);
    font: 15px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC",
          "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
  }
  .wrap { max-width: 1080px; margin: 0 auto; padding: 24px 16px 64px; }
  h1 { font-size: 22px; margin: 0 0 4px; }
  h2 { font-size: 17px; margin: 32px 0 12px; }
  .sub { color: var(--muted); font-size: 13px; }
  .card {
    background: var(--card); border: 1px solid var(--border);
    border-radius: 10px; padding: 16px 18px; margin-top: 16px;
  }
  dl.summary { display: flex; flex-wrap: wrap; gap: 8px 28px; margin: 0; }
  dl.summary div { display: flex; gap: 8px; align-items: baseline; }
  dt { color: var(--muted); font-size: 13px; }
  dd { margin: 0; font-family: var(--mono); font-weight: 600; }
  .budget-bar { width: 100%; height: 12px; margin-top: 14px; border-radius: 6px;
    background: var(--bg); border: 1px solid var(--border); overflow: hidden; }
  .budget-fill { height: 100%; background: var(--accent); }
  .table-scroll { overflow-x: auto; }
  table { width: 100%; border-collapse: collapse; font-size: 13.5px; min-width: 820px; }
  th, td { padding: 8px 10px; text-align: right; border-bottom: 1px solid var(--border); white-space: nowrap; }
  th { color: var(--muted); font-weight: 600; font-size: 12.5px; }
  td.l, th.l { text-align: left; }
  tr.rejected { background: var(--bad-bg); }
  tr.rejected td { color: var(--bad-fg); }
  .tag { display: inline-block; padding: 1px 8px; border-radius: 999px; font-size: 12px; font-weight: 600; }
  .tag.ok { background: var(--ok-bg); color: var(--ok-fg); }
  .tag.bad { background: var(--bad-bg); color: var(--bad-fg); }
  .mono { font-family: var(--mono); }
  ul.findings { margin: 0; padding-left: 20px; }
  ul.findings li { margin: 8px 0; }
  .kind { display: inline-block; padding: 1px 8px; border-radius: 4px; font-size: 12px;
    font-weight: 600; background: var(--warn-bg); color: var(--warn-fg); margin-right: 6px; }
  .none { color: var(--muted); }
  footer { margin-top: 36px; color: var(--muted); font-size: 12.5px; }
  #filter { margin: 0 0 10px; font-size: 13px; color: var(--muted); display: flex; gap: 14px; }
  #filter label { cursor: pointer; user-select: none; }
</style>
</head>
<body>
<div class="wrap">
  <h1>{{.Title}}</h1>
  <p class="sub">数据集：{{.Dataset}}（{{.RecordN}} 条记录） · 生成时间：{{.Generated.Format "2006-01-02 15:04:05"}}</p>

  <div class="card">
    <dl class="summary">
      <div><dt>隐私预算上限 ε</dt><dd>{{f4 .Summary.Budget}}</dd></div>
      <div><dt>已消耗 ε（序列组合求和）</dt><dd>{{f4 .Summary.Spent}}</dd></div>
      <div><dt>剩余 ε</dt><dd>{{f4 .Summary.Remaining}}</dd></div>
      <div><dt>查询总数</dt><dd>{{.Summary.Total}}</dd></div>
      <div><dt>已执行</dt><dd>{{.Summary.Accepted}}</dd></div>
      <div><dt>已拒绝</dt><dd>{{.Summary.Rejected}}</dd></div>
    </dl>
    <div class="budget-bar"><div class="budget-fill" style="width: {{pct (div .Summary.Spent .Summary.Budget)}}"></div></div>
    <p class="sub" style="margin:8px 0 0">预算使用率 {{pct (div .Summary.Spent .Summary.Budget)}}（基础序列组合：ε_total = Σ ε_i）</p>
  </div>

  <h2>查询明细</h2>
  <div id="filter">
    <label><input type="checkbox" id="hideRejected"> 隐藏被拒绝的查询</label>
  </div>
  <div class="card table-scroll" style="margin-top:0">
    <table id="results">
      <thead>
        <tr>
          <th class="l">编号</th><th class="l">查询</th><th>ε</th><th>敏感度 Δf</th>
          <th>噪声尺度 b</th><th>真实值</th><th>加噪结果</th><th>绝对误差</th>
          <th>剩余预算</th><th class="l">状态</th>
        </tr>
      </thead>
      <tbody>
        {{- range .Results}}
        <tr{{if .Rejected}} class="rejected"{{end}}>
          <td class="l mono">{{.Query.ID}}</td>
          <td class="l">{{.Query.Description}}</td>
          <td class="mono">{{f4 .Query.Epsilon}}</td>
          <td class="mono">{{f2 .Query.EffectiveSensitivity}}</td>
          {{- if .Rejected}}
          <td class="mono">—</td><td class="mono">{{.TrueCount}}</td><td class="mono">—</td><td class="mono">—</td>
          <td class="mono">{{f4 .RemainingAfter}}</td>
          <td class="l"><span class="tag bad">已拒绝</span> <span class="sub">{{.RejectReason}}</span></td>
          {{- else}}
          <td class="mono">{{f4 .Scale}}</td>
          <td class="mono">{{.TrueCount}}</td>
          <td class="mono"><strong>{{f4 .NoisyCount}}</strong></td>
          <td class="mono">{{err .}}</td>
          <td class="mono">{{f4 .RemainingAfter}}</td>
          <td class="l"><span class="tag ok">已执行</span></td>
          {{- end}}
        </tr>
        {{- end}}
      </tbody>
    </table>
  </div>

  <h2>组合分析（低效查询模式）</h2>
  <div class="card" style="margin-top:0">
    {{- if .Findings}}
    <ul class="findings">
      {{- range .Findings}}
      <li><span class="kind">{{.Kind}}</span>{{.Detail}}
        {{- if gt .Wasted 0.0}} <span class="mono sub">（可节省 ε ≈ {{f4 .Wasted}}）</span>{{end}}</li>
      {{- end}}
    </ul>
    {{- else}}
    <p class="none" style="margin:0">未发现明显的低效查询模式。</p>
    {{- end}}
  </div>

  <footer>
    机制：Laplace 机制，噪声尺度 b = Δf/ε，理论方差 2b²；组合方式：基础序列组合（ε 累加）。
    本页面为单一静态文件，全部数据内嵌，无外部依赖。
  </footer>
</div>
<script>
  document.getElementById('hideRejected').addEventListener('change', function (e) {
    document.querySelectorAll('#results tbody tr.rejected').forEach(function (tr) {
      tr.style.display = e.target.checked ? 'none' : '';
    });
  });
</script>
</body>
</html>
`
