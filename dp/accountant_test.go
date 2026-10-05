package dp

import (
	"errors"
	"math"
	"testing"
)

// TestAccountantAccumulation 验证多次查询下预算按序列组合正确累加。
func TestAccountantAccumulation(t *testing.T) {
	a, err := NewAccountant(2.0)
	if err != nil {
		t.Fatal(err)
	}
	for i, eps := range []float64{0.3, 0.5, 0.7} {
		if err := a.Spend(eps); err != nil {
			t.Fatalf("第 %d 次记账失败: %v", i+1, err)
		}
	}
	if got := a.Spent(); math.Abs(got-1.5) > 1e-12 {
		t.Errorf("累计消耗 = %v，期望 1.5", got)
	}
	if got := a.Remaining(); math.Abs(got-0.5) > 1e-12 {
		t.Errorf("剩余预算 = %v，期望 0.5", got)
	}
}

// TestAccountantRejectsOverBudget 验证超预算查询被拒绝且不消耗预算。
func TestAccountantRejectsOverBudget(t *testing.T) {
	a, _ := NewAccountant(1.0)
	if err := a.Spend(0.6); err != nil {
		t.Fatal(err)
	}
	err := a.Spend(0.5) // 0.6+0.5=1.1 > 1.0，必须拒绝
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("应返回 ErrBudgetExhausted，得到 %v", err)
	}
	if got := a.Spent(); math.Abs(got-0.6) > 1e-12 {
		t.Errorf("被拒绝的查询不应消耗预算，已用 = %v，期望 0.6", got)
	}
}

// TestAccountantExactBudget 验证恰好用尽预算的查询被接受。
func TestAccountantExactBudget(t *testing.T) {
	a, _ := NewAccountant(1.0)
	if err := a.Spend(0.4); err != nil {
		t.Fatal(err)
	}
	if err := a.Spend(0.6); err != nil {
		t.Fatalf("恰好用尽的查询应被接受: %v", err)
	}
	if got := a.Remaining(); math.Abs(got) > 1e-12 {
		t.Errorf("剩余 = %v，期望 0", got)
	}
	if err := a.Spend(0.001); err == nil {
		t.Error("预算用尽后任何正 ε 查询都应被拒绝")
	}
}

// TestAccountantInvalid 验证非法预算与非法消耗被拒绝。
func TestAccountantInvalid(t *testing.T) {
	if _, err := NewAccountant(0); err == nil {
		t.Error("零预算应报错")
	}
	if _, err := NewAccountant(-1); err == nil {
		t.Error("负预算应报错")
	}
	a, _ := NewAccountant(1.0)
	if err := a.Spend(0); err == nil {
		t.Error("零 ε 消耗应报错")
	}
	if err := a.Spend(-0.5); err == nil {
		t.Error("负 ε 消耗应报错")
	}
}
