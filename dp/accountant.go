package dp

import "fmt"

// Accountant 跟踪一个数据集上的隐私预算（ε）消耗。
// 规则：每次成功执行的查询消耗其声明的 ε；被拒绝的查询不消耗预算；
// 当 已消耗 + 本次 ε > 预算上限 时拒绝查询（不允许超支）。
type Accountant struct {
	budget float64
	spent  float64
}

// NewAccountant 创建预算上限为 budget 的记账器。
func NewAccountant(budget float64) (*Accountant, error) {
	if budget <= 0 {
		return nil, fmt.Errorf("dp: 隐私总量子预算必须为正，得到 %v", budget)
	}
	return &Accountant{budget: budget}, nil
}

// Budget 返回预算上限。
func (a *Accountant) Budget() float64 { return a.budget }

// Spent 返回已消耗的 ε（按基础序列组合定理累加）。
func (a *Accountant) Spent() float64 { return a.spent }

// Remaining 返回剩余预算。
func (a *Accountant) Remaining() float64 { return a.budget - a.spent }

// CanSpend 判断消耗 eps 是否会导致超支。
func (a *Accountant) CanSpend(eps float64) bool {
	return eps > 0 && a.spent+eps <= a.budget
}

// Spend 记账一次查询消耗；若会导致超支则返回错误且不记账。
func (a *Accountant) Spend(eps float64) error {
	if eps <= 0 {
		return fmt.Errorf("dp: 单次查询 ε 必须为正，得到 %v", eps)
	}
	if !a.CanSpend(eps) {
		return fmt.Errorf("%w：本次需 ε=%.4g，剩余 %.4g（上限 %.4g，已用 %.4g）",
			ErrBudgetExhausted, eps, a.Remaining(), a.budget, a.spent)
	}
	a.spent += eps
	return nil
}
