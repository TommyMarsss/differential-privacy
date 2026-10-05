package dp

import (
	"math"
	"math/rand/v2"
	"strings"
	"testing"
)

// randForTest 返回固定种子的随机源，使引擎测试可复现。
func randForTest() RNG { return rand.New(rand.NewPCG(1, 2)) }

func TestNewAccountantRejectsBadBudget(t *testing.T) {
	for _, total := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := NewAccountant(total); err == nil {
			t.Errorf("NewAccountant(%v) 应拒绝非正/非有限预算", total)
		}
	}
}

// TestAccountantSequentialSpend 验证多次查询下预算按顺序组合定理线性累加。
func TestAccountantSequentialSpend(t *testing.T) {
	acc, _ := NewAccountant(1.0)
	spends := []float64{0.2, 0.1, 0.1, 0.1, 0.1, 0.2}
	wantUsed := []float64{0.2, 0.3, 0.4, 0.5, 0.6, 0.8}
	wantRem := []float64{0.8, 0.7, 0.6, 0.5, 0.4, 0.2}
	for i, eps := range spends {
		if err := acc.Spend(0.0); err == nil {
			t.Fatalf("#%d: ε=0 应被拒绝", i)
		}
		if err := acc.Spend(eps); err != nil {
			t.Fatalf("#%d Spend(%.1f) 意外失败: %v", i, eps, err)
		}
		if math.Abs(acc.Used()-wantUsed[i]) > 1e-12 {
			t.Errorf("#%d 已用预算 = %.6f, want %.6f", i, acc.Used(), wantUsed[i])
		}
		if math.Abs(acc.Remaining()-wantRem[i]) > 1e-12 {
			t.Errorf("#%d 剩余预算 = %.6f, want %.6f", i, acc.Remaining(), wantRem[i])
		}
	}
	if acc.Spends() != 6 {
		t.Errorf("Spends() = %d, want 6", acc.Spends())
	}
}

// TestAccountantExactExhaustion 验证预算“恰好”用尽时允许（≤ 边界），
// 再多申请任意正数即被拒绝，且被拒后状态不变。
func TestAccountantExactExhaustion(t *testing.T) {
	acc, _ := NewAccountant(0.3)
	if err := acc.Spend(0.1); err != nil {
		t.Fatal(err)
	}
	if err := acc.Spend(0.2); err != nil {
		t.Fatalf("预算恰好用尽的查询应被允许: %v", err)
	}
	used := acc.Used()
	err := acc.Spend(1e-9)
	if err == nil {
		t.Fatal("预算用尽后申请 1e-9 应被拒绝")
	}
	if !strings.Contains(err.Error(), "exceeding budget") {
		t.Errorf("拒绝原因应明确报告预算超限: %v", err)
	}
	if acc.Used() != used {
		t.Errorf("被拒绝后已用预算不得改变: before=%.12f after=%.12f", used, acc.Used())
	}
	if acc.Spends() != 2 {
		t.Errorf("被拒绝的查询不记账，Spends() = %d, want 2", acc.Spends())
	}
}

// TestAccountantFloatTolerance 0.3×3 vs 0.9 的累加误差不应导致误拒。
func TestAccountantFloatTolerance(t *testing.T) {
	acc, _ := NewAccountant(0.9)
	if err := acc.Spend(0.3); err != nil {
		t.Fatal(err)
	}
	if err := acc.Spend(0.3); err != nil {
		t.Fatal(err)
	}
	if err := acc.Spend(0.3); err != nil {
		t.Fatalf("0.3+0.3+0.3 恰好等于预算（浮点容差内），不应拒绝: %v", err)
	}
}

// TestAccountantRejectAfterReject 被拒查询不影响后续能用剩余预算执行的查询。
func TestAccountantRejectDoesNotConsume(t *testing.T) {
	acc, _ := NewAccountant(0.5)
	_ = acc.Spend(0.4)
	var bigErr error
	if err := acc.Spend(0.3); err != nil {
		bigErr = err
	}
	if bigErr == nil {
		t.Fatal("0.4+0.3 > 0.5 应被拒绝")
	}
	// 剩余 0.1 仍可使用。
	if err := acc.Spend(0.1); err != nil {
		t.Errorf("拒绝 0.3 后剩余 0.1 仍应可用, got %v", err)
	}
}

func TestAccountantCheckDoesNotMutate(t *testing.T) {
	acc, _ := NewAccountant(1.0)
	ok, _ := acc.Check(2.0)
	if ok {
		t.Error("Check(2.0) 应为不可批准")
	}
	if acc.Used() != 0 || acc.Spends() != 0 {
		t.Error("Check 不得改变记账器状态")
	}
}

func TestEngineRejectsOverBudgetAndDoesNotPublish(t *testing.T) {
	ds := &Dataset{Name: "t", Records: []Record{{"g": "a"}, {"g": "b"}}}
	rng := randForTest()
	engine, _ := NewEngine(0.3, rng)
	rep := engine.Run("t", ds, []Query{
		{Name: "q0", Epsilon: 0.2, Predicate: func(r Record) bool { return true }},
		{Name: "q1", Epsilon: 0.2, Predicate: func(r Record) bool { return true }},
		{Name: "q2", Epsilon: 0.1, Predicate: func(r Record) bool { return true }},
	})

	if math.Abs(rep.BudgetUsed-0.3) > 1e-12 {
		t.Errorf("BudgetUsed = %v, want 0.3", rep.BudgetUsed)
	}
	if rep.Accepted != 2 || rep.Rejected != 1 {
		t.Errorf("accepted/rejected = %d/%d, want 2/1", rep.Accepted, rep.Rejected)
	}
	r := rep.Results[1]
	if r.Accepted {
		t.Fatal("0.2+0.2>0.3 的查询应被拒绝")
	}
	if !strings.Contains(r.RejectReason, "exceeding budget") {
		t.Errorf("拒绝原因应明确报告超预算: %q", r.RejectReason)
	}
	if r.Noise != 0 || r.NoisyValue != 0 || r.PublishedValue != 0 {
		t.Errorf("被拒查询不得生成噪声/结果: noise=%v noisy=%v pub=%v",
			r.Noise, r.NoisyValue, r.PublishedValue)
	}
	if r.Scale != 0 {
		t.Errorf("被拒查询不应记录噪声尺度, got %v", r.Scale)
	}
	if math.Abs(r.UsedAfter-0.2) > 1e-12 || math.Abs(r.RemainingAfter-0.1) > 1e-12 {
		t.Errorf("被拒后预算快照错误: usedAfter=%v remaining=%v", r.UsedAfter, r.RemainingAfter)
	}
	// 被拒后第三笔 0.1 应能用尽剩余预算。
	if !rep.Results[2].Accepted {
		t.Errorf("拒绝后可用剩余预算继续: %s", rep.Results[2].RejectReason)
	}
}

func TestEngineInvalidEpsilon(t *testing.T) {
	ds := &Dataset{Name: "t", Records: []Record{{"g": "a"}}}
	engine, _ := NewEngine(1.0, randForTest())
	rep := engine.Run("t", ds, []Query{
		{Name: "bad", Epsilon: 0, Predicate: func(r Record) bool { return true }},
	})
	if rep.Results[0].Accepted {
		t.Fatal("ε=0 必须被拒绝")
	}
	if !strings.Contains(rep.Results[0].RejectReason, "epsilon") {
		t.Errorf("拒绝原因应指出 ε 非法: %q", rep.Results[0].RejectReason)
	}
	if rep.BudgetUsed != 0 {
		t.Errorf("非法查询不消耗预算, used=%v", rep.BudgetUsed)
	}
}

// TestEngineNoiseMovesResult 注入固定值会被拒绝；这里验证加噪结果 =
// 真实值 + Laplace 噪声、发布值为取整截断后的非负整数。
func TestEnginePostProcessing(t *testing.T) {
	ds := &Dataset{Name: "t", Records: make([]Record, 10)} // 10 条，谓词全命中
	engine, _ := NewEngine(10.0, randForTest())
	rep := engine.Run("t", ds, []Query{
		{Name: "q", Epsilon: 5.0, Predicate: func(r Record) bool { return true }},
	})
	r := rep.Results[0]
	if !r.Accepted {
		t.Fatal(r.RejectReason)
	}
	if math.Abs(r.NoisyValue-(float64(r.TrueValue)+r.Noise)) > 1e-12 {
		t.Errorf("加噪值不等于 真实值+噪声: %v vs %v", r.NoisyValue, float64(r.TrueValue)+r.Noise)
	}
	if r.PublishedValue < 0 {
		t.Error("发布值被截断为非负")
	}
	wantScale := 1.0 / 5.0
	if math.Abs(r.Scale-wantScale) > 1e-12 {
		t.Errorf("Scale = %v, want %v", r.Scale, wantScale)
	}
}

func TestEngineSensitivity(t *testing.T) {
	ds := &Dataset{Name: "t", Records: make([]Record, 4)}
	engine, _ := NewEngine(10.0, randForTest())
	rep := engine.Run("t", ds, []Query{
		{Name: "s2", Epsilon: 0.5, Sensitivity: 2, Predicate: func(r Record) bool { return true }},
	})
	r := rep.Results[0]
	if math.Abs(r.Scale-4.0) > 1e-12 {
		t.Errorf("Δ=2, ε=0.5 时 b=4, got %v", r.Scale)
	}
}

func TestNewEngineBadArgs(t *testing.T) {
	if _, err := NewEngine(0, randForTest()); err == nil {
		t.Error("总预算非法时应返回错误")
	}
	if _, err := NewEngine(1.0, nil); err == nil {
		t.Error("nil 随机源应返回错误")
	}
}
