package dp

import (
	"fmt"
	"strings"
)

// Finding 是组合分析发现的一条低效查询模式告警。
type Finding struct {
	Kind     string   // 模式类型
	Detail   string   // 人类可读说明
	QueryIDs []string // 涉及的查询编号
	Wasted   float64  // 估算可节省的 ε
}

// 组合分析识别的低效模式类型。
const (
	// FindingDuplicate：同一语义键被重复查询。重复查询同一谓词会按组合定理
	// 重复消耗预算，而首次结果已可复用（或应以一次更大 ε 的查询替代）。
	FindingDuplicate = "重复查询"
	// FindingTotalThenParts：先查总数、再查其各子集（或反之）。子集计数之和
	// 即为总数，其中一次查询的预算属于浪费。
	FindingTotalThenParts = "总数+子集冗余"
	// FindingRefinement：查询是对已查集合的细分子集查询，二者高度相关，
	// 预算按序列组合全额叠加，应考虑用一次查询加后处理代替。
	FindingRefinement = "子集细化查询"
)

// AnalyzeComposition 在查询计划层面识别会导致隐私预算快速耗尽的低效模式。
// 依据 Query.Key / Query.RefinesKey 元数据（谓词函数本身不可比较）。
func AnalyzeComposition(queries []Query) []Finding {
	var findings []Finding

	// 1. 重复语义键。
	seen := map[string]Query{}
	for _, q := range queries {
		if q.Key == "" {
			continue
		}
		if prev, ok := seen[q.Key]; ok {
			findings = append(findings, Finding{
				Kind: FindingDuplicate,
				Detail: fmt.Sprintf("%s 与 %s 查询同一谓词（%q），重复消耗 ε=%.4g；可复用首次结果。",
					q.ID, prev.ID, q.Key, q.Epsilon),
				QueryIDs: []string{prev.ID, q.ID},
				Wasted:   q.Epsilon,
			})
		} else {
			seen[q.Key] = q
		}
	}

	// 2. 显式声明的细化关系（RefinesKey）。
	for _, q := range queries {
		if q.RefinesKey == "" {
			continue
		}
		if parent, ok := seen[q.RefinesKey]; ok {
			findings = append(findings, Finding{
				Kind: FindingRefinement,
				Detail: fmt.Sprintf("%s 是 %s 的子集细化查询（%q ⊆ %q），两次查询的 ε 将全额叠加。",
					q.ID, parent.ID, q.Key, parent.Key),
				QueryIDs: []string{parent.ID, q.ID},
				Wasted:   0, // 细化查询本身可能必要，仅提示叠加风险
			})
		}
	}

	// 3. 总数 + 覆盖同一维度的子集划分：子集之和即总数。
	// 约定：Key 形如 "count:*" 为总数，"count:<dim>=<value>" 为子集。
	totalReported := false
	for _, q := range queries {
		if q.Key != "count:*" || totalReported {
			continue
		}
		dimParts := map[string][]Query{}
		for _, p := range queries {
			dim, ok := subsetDimension(p.Key)
			if ok {
				dimParts[dim] = append(dimParts[dim], p)
			}
		}
		for dim, parts := range dimParts {
			if len(parts) < 2 {
				continue
			}
			ids := []string{q.ID}
			wasted := q.Epsilon
			for _, p := range parts {
				ids = append(ids, p.ID)
			}
			findings = append(findings, Finding{
				Kind: FindingTotalThenParts,
				Detail: fmt.Sprintf("%s 查询总数，而 %s 已按维度 %q 查询各子集；子集之和即总数，总数查询的 ε=%.4g 属于浪费。",
					q.ID, ids[1:], dim, q.Epsilon),
				QueryIDs: ids,
				Wasted:   wasted,
			})
			totalReported = true
		}
	}

	return findings
}

// subsetDimension 解析 "count:<dim>=<value>" 形式的语义键，返回维度名。
// 维度名必须是简单标识符（字母/数字/下划线），值中不得再含 "=" 或 ","，
// 以排除 "count:age>=40"、"count:dept=x,age>=y" 等复合条件——它们不构成
// 对数据的划分，不能套用"子集之和即总数"的规则。
// 非子集键（如 "count:*"）返回 ok=false。
func subsetDimension(key string) (dim string, ok bool) {
	rest, found := strings.CutPrefix(key, "count:")
	if !found || rest == "*" {
		return "", false
	}
	dim, val, found := strings.Cut(rest, "=")
	if !found || dim == "" || val == "" {
		return "", false
	}
	for _, c := range dim {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return "", false
		}
	}
	if strings.ContainsAny(val, "=,") {
		return "", false
	}
	return dim, true
}
