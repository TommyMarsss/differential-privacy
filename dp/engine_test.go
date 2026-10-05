package dp

import (
	"math"
	"math/rand/v2"
	"testing"
)

func testDataset() Dataset {
	return Dataset{
		Name: "test",
		Records: []Record{
			{"dept": "eng", "age": "30"},
			{"dept": "eng", "age": "45"},
			{"dept": "sales", "age": "28"},
			{"dept": "sales", "age": "52"},
			{"dept": "hr", "age": "41"},
		},
	}
}

func engQuery(id string, eps float64) Query {
	return Query{
		ID: id, Epsilon: eps, Key: "count:dept=eng",
		Predicate: func(r Record) bool { return r["dept"] == "eng" },
	}
}

// TestEngineCountsAndNoise 验证真实计数正确，且加噪结果 = 真实值 + 噪声。
func TestEngineCountsAndNoise(t *testing.T) {
	acct, _ := NewAccountant(10)
	e := NewEngine(testDataset(), acct, rand.New(rand.NewPCG(1, 1)))
	res := e.Execute(engQuery("Q1", 0.5))
	if res.Rejected {
		t.Fatalf("查询不应被拒绝: %s", res.RejectReason)
	}
	if res.TrueCount != 2 {
		t.Errorf("真实计数 = %d，期望 2", res.TrueCount)
	}
	if math.Abs(res.NoisyCount-(float64(res.TrueCount)+res.Noise)) > 1e-12 {
		t.Error("加噪结果应等于真实值加噪声")
	}
	if math.Abs(res.Scale-2.0) > 1e-12 { // b = Δf/ε = 1/0.5
		t.Errorf("噪声尺度 = %v，期望 2.0", res.Scale)
	}
}

// TestEngineRejectsOverBudget 验证引擎在预算不足时拒绝查询、不消耗预算、并明确报告原因。
func TestEngineRejectsOverBudget(t *testing.T) {
	acct, _ := NewAccountant(1.0)
	e := NewEngine(testDataset(), acct, rand.New(rand.NewPCG(2, 2)))

	r1 := e.Execute(engQuery("Q1", 0.8))
	if r1.Rejected {
		t.Fatalf("Q1 不应被拒绝: %s", r1.RejectReason)
	}
	r2 := e.Execute(engQuery("Q2", 0.5)) // 0.8+0.5 > 1.0
	if !r2.Rejected {
		t.Fatal("Q2 超预算，应被拒绝")
	}
	if r2.RejectReason == "" {
		t.Error("被拒绝的查询必须给出明确原因")
	}
	if got := acct.Spent(); math.Abs(got-0.8) > 1e-12 {
		t.Errorf("被拒绝后已用预算 = %v，期望仍为 0.8", got)
	}
	// 后续小预算查询仍应可执行。
	r3 := e.Execute(engQuery("Q3", 0.2))
	if r3.Rejected {
		t.Fatalf("Q3 恰好用尽预算，应被接受: %s", r3.RejectReason)
	}
}

// TestEngineComposition 验证序列组合：总隐私损失等于各次 ε 之和。
func TestEngineComposition(t *testing.T) {
	acct, _ := NewAccountant(5)
	e := NewEngine(testDataset(), acct, rand.New(rand.NewPCG(3, 3)))
	queries := []Query{
		engQuery("Q1", 0.5), engQuery("Q2", 1.0), engQuery("Q3", 0.25),
	}
	results := e.Run(queries)
	total := TotalEpsilon(results)
	if math.Abs(total-1.75) > 1e-12 {
		t.Errorf("组合隐私损失 = %v，期望 1.75", total)
	}
	if math.Abs(acct.Spent()-total) > 1e-12 {
		t.Error("记账器累计消耗应与组合隐私损失一致")
	}
}

// TestEngineInvalidEpsilon 验证非法 ε 的查询被拒绝且不消耗预算。
func TestEngineInvalidEpsilon(t *testing.T) {
	acct, _ := NewAccountant(5)
	e := NewEngine(testDataset(), acct, rand.New(rand.NewPCG(4, 4)))
	res := e.Execute(engQuery("Q1", -1))
	if !res.Rejected {
		t.Fatal("负 ε 查询应被拒绝")
	}
	if acct.Spent() != 0 {
		t.Error("非法查询不应消耗预算")
	}
}

// TestEngineSensitivity 验证自定义敏感度影响噪声尺度。
func TestEngineSensitivity(t *testing.T) {
	acct, _ := NewAccountant(10)
	e := NewEngine(testDataset(), acct, rand.New(rand.NewPCG(5, 5)))
	q := engQuery("Q1", 0.5)
	q.Sensitivity = 4
	res := e.Execute(q)
	if math.Abs(res.Scale-8.0) > 1e-12 { // b = 4/0.5
		t.Errorf("噪声尺度 = %v，期望 8.0", res.Scale)
	}
}

// TestAnalyzeComposition 验证低效模式识别：重复查询、总数+子集、细化查询。
func TestAnalyzeComposition(t *testing.T) {
	queries := []Query{
		{ID: "Q1", Key: "count:*", Epsilon: 0.5},
		{ID: "Q2", Key: "count:dept=eng", Epsilon: 0.5, RefinesKey: "count:*"},
		{ID: "Q3", Key: "count:dept=sales", Epsilon: 0.5, RefinesKey: "count:*"},
		{ID: "Q4", Key: "count:*", Epsilon: 0.5}, // 重复
	}
	findings := AnalyzeComposition(queries)

	kinds := map[string]int{}
	for _, f := range findings {
		kinds[f.Kind]++
	}
	if kinds[FindingDuplicate] != 1 {
		t.Errorf("应发现 1 条重复查询告警，得到 %d", kinds[FindingDuplicate])
	}
	if kinds[FindingTotalThenParts] != 1 { // 仅对首个总数键报告一次
		t.Errorf("应发现 1 条总数+子集告警，得到 %d", kinds[FindingTotalThenParts])
	}
	if kinds[FindingRefinement] != 2 {
		t.Errorf("应发现 2 条细化查询告警，得到 %d", kinds[FindingRefinement])
	}
}

// TestAnalyzeCompositionClean 验证无冗余的查询计划不产生告警。
func TestAnalyzeCompositionClean(t *testing.T) {
	queries := []Query{
		{ID: "Q1", Key: "count:dept=eng", Epsilon: 0.5},
		{ID: "Q2", Key: "count:age>=40", Epsilon: 0.5},
	}
	if findings := AnalyzeComposition(queries); len(findings) != 0 {
		t.Errorf("不应有告警，得到 %v", findings)
	}
}
