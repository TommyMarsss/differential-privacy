package dp

import (
	"fmt"
	"math"
)

// Accountant 是纯 ε-DP 的隐私预算记账器。
//
// 记账规则（简单顺序组合，basic sequential composition）：
// 在同一数据集上依次执行满足 ε_1,…,ε_k-DP 的机制 M_1,…,M_k，
// 其组合机制 M(x)=(M_1(x),…,M_k(x)) 满足 (Σ ε_i)-DP。
// 因此记账器对每笔已批准查询的 ε 做线性累加，无折扣、无重置。
//
// 拒绝规则（先记账后执行）：当 used + epsilon > total 时，查询在注入噪声
// 之前即被拒绝——不消耗预算、不生成噪声、不发布任何结果。
type Accountant struct {
	total  float64 // 预算上限 ε_total
	used   float64 // 累计已消耗 ε
	spends int     // 已成功记账的查询数
}

// NewAccountant 创建预算上限为 total 的记账器，total 必须为正有限数。
func NewAccountant(total float64) (*Accountant, error) {
	if !(total > 0) || math.IsNaN(total) || math.IsInf(total, 0) {
		return nil, fmt.Errorf("dp: total budget must be a positive finite number, got %v", total)
	}
	return &Accountant{total: total}, nil
}

// Total 返回预算上限。
func (a *Accountant) Total() float64 { return a.total }

// Used 返回累计已消耗的 ε。
func (a *Accountant) Used() float64 { return a.used }

// Remaining 返回剩余预算 total - used，预算耗尽时返回 0。
func (a *Accountant) Remaining() float64 {
	r := a.total - a.used
	if r < 0 {
		return 0
	}
	return r
}

// Spends 返回已成功记账的查询笔数。
func (a *Accountant) Spends() int { return a.spends }

// Check 判断申请 epsilon 的查询当前是否可被批准。
// 第二个返回值为拒绝原因；批准时原因为空。该方法不改变记账器状态。
func (a *Accountant) Check(epsilon float64) (bool, string) {
	if !(epsilon > 0) || math.IsNaN(epsilon) || math.IsInf(epsilon, 0) {
		return false, fmt.Sprintf(
			"dp: requested epsilon must be a positive finite number, got %v", epsilon)
	}
	projected := a.used + epsilon
	if projected > a.total+budgetEpsilon {
		return false, fmt.Sprintf(
			"requested epsilon %.6f would raise total spend from %.6f to %.6f, "+
				"exceeding budget %.6f (remaining %.6f)",
			epsilon, a.used, projected, a.total, a.Remaining())
	}
	return true, ""
}

// Spend 尝试为查询记账 epsilon：成功则累加并返回 nil；超预算或参数非法时
// 返回错误且记账器状态保持不变（拒绝的查询不花一分预算）。
func (a *Accountant) Spend(epsilon float64) error {
	if ok, reason := a.Check(epsilon); !ok {
		return fmt.Errorf("dp: budget exceeded: %s", reason)
	}
	a.used += epsilon
	a.spends++
	return nil
}

// budgetEpsilon 是浮点比较容差：used+epsilon 仅在数值意义上略微超出 total
// （如 0.3+0.3+0.3+0.1 类累加误差）时仍视为合法，避免误拒。
const budgetEpsilon = 1e-12
