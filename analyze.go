package dp

import (
	"fmt"
	"math/big"
	"math/bits"
	"sort"
	"strings"
)

// Finding 描述一种“查询设计本身导致预算快速耗尽”的低效模式。
type Finding struct {
	Kind          string  `json:"kind"`
	Severity      string  `json:"severity"` // high / medium / low
	QueryIndices  []int   `json:"queryIndices"`
	Message       string  `json:"message"`
	WastedEpsilon float64 `json:"wastedEpsilon"`
}

// 可识别的三类模式：
//
//  1. duplicate（重复发布）：两次已接受查询命中完全相同的记录集合，
//     后一次的答案直接复用上一次即可，其 ε 纯属浪费。
//  2. derivable（可后处理推导）：某次计数可由若干两两不相交的较早查询
//     逐一无重叠求和得到。求和是后处理（post-processing），不耗预算；
//     “先查总数、再拆查各子集”时总数那一问即属此类。
//  3. histogram-opportunity（可合并为直方图）：三个及以上两两不相交的
//     子集计数被拆成独立查询。合并为一次向量（直方图）发布时，一条记录
//     至多落入一个桶，向量的 L1 敏感度仍为 1，可在同等噪声尺度下
//     显著节省预算。
//
// 这些分析只使用命中集合与元数据，不使用噪声值，因此与 DP 保证无关。
const (
	kindDuplicate = "duplicate"
	kindDerivable = "derivable"
	kindHistogram = "histogram-opportunity"
	sevHigh       = "high"
	sevMedium     = "medium"
)

// Analyze 对一次 Run 中已接受的查询做模式分析。sels[i] 为第 i 个查询
// 命中记录的位集（被拒绝查询对应 nil）。
func Analyze(queries []Query, results []Result, sels []selection) []Finding {
	var findings []Finding

	accepted := make([]int, 0)
	for i, r := range results {
		if r.Accepted {
			accepted = append(accepted, i)
		}
	}

	// --- 模式 3：可合并直方图（贪心枚举两两不相交的查询组，至少 3 个） ---
	used := map[int]bool{}
	for _, seed := range accepted {
		if used[seed] {
			continue
		}
		group := []int{seed}
		groupUnion := new(big.Int).Set(sels[seed].set)
		for _, j := range accepted {
			if j <= seed || used[j] {
				continue
			}
			if sels[j].set.Sign() == 0 {
				continue // 空集计数（恒为 0）无信息量，跳过
			}
			if disjoint(groupUnion, sels[j].set) {
				group = append(group, j)
				groupUnion.Or(groupUnion, sels[j].set)
			}
		}
		if len(group) >= 3 {
			for _, g := range group {
				used[g] = true
			}
			findings = append(findings, histogramFinding(queries, results, group))
		}
	}

	// --- 模式 1、2：按查询顺序逐个与较早查询比对 ---
	for _, i := range accepted {
		// 模式 1：与任一较早查询命中集合完全相同。
		for _, j := range accepted {
			if j >= i {
				break
			}
			if equalSet(sels[i].set, sels[j].set) {
				findings = append(findings, Finding{
					Kind:          kindDuplicate,
					Severity:      sevHigh,
					QueryIndices:  []int{j, i},
					WastedEpsilon: results[i].Epsilon,
					Message: fmt.Sprintf(
						"查询 %q 与已执行的 %q 选择了完全相同的记录集合，重复发布；"+
							"直接复用上次的加噪结果即可，本次 ε=%.4f 纯属浪费。",
						queries[i].Name, queries[j].Name, results[i].Epsilon),
				})
				break
			}
		}

		// 模式 2：可由较早的两两不相交查询无重叠求和得到。
		if members := derivableFrom(i, accepted, sels); len(members) >= 2 {
			findings = append(findings, Finding{
				Kind:          kindDerivable,
				Severity:      sevHigh,
				QueryIndices:  append(members, i),
				WastedEpsilon: results[i].Epsilon,
				Message: fmt.Sprintf(
					"查询 %q 的计数可由 %s 的计数逐一无重叠求和得到（后处理，不耗预算），"+
						"单独发布它多花了 ε=%.4f；这正是'先查总数再查各子集'式的低效组合。",
					queries[i].Name, qNameList(queries, members), results[i].Epsilon),
			})
		}
	}

	if findings == nil {
		return []Finding{}
	}
	return findings
}

// derivableFrom 寻找一组早于 target、两两不相交且并集恰等于 target 命中
// 集合的查询下标（精确覆盖）。采用 Algorithm X 风格的 DFS：每步选择
// “未覆盖的最小记录位”，只在包含该位的候选集合上分支，配合查询下标
// 递增约束避免排列重复。该问题一般意义上是 NP 的，但预算约束下实际查询
// 数量很小；超过搜索节点预算时保守地返回 nil（宁漏报不误报）。
func derivableFrom(target int, accepted []int, sels []selection) []int {
	candidates := make([]int, 0)
	for _, j := range accepted {
		if j < target && sels[j].set.Sign() > 0 && subsetOf(sels[j].set, sels[target].set) {
			candidates = append(candidates, j)
		}
	}

	var chosen []int
	visited := 0
	const nodeBudget = 100000

	var dfs func(firstCand int, uncovered *big.Int) bool
	dfs = func(firstCand int, uncovered *big.Int) bool {
		if uncovered.Sign() == 0 {
			return len(chosen) >= 2
		}
		visited++
		if visited > nodeBudget {
			return false
		}
		p := uncovered
		low := new(big.Int).And(p, new(big.Int).Neg(p)) // 最低有效位
		bit := lowestSetBit(low)
		for n := firstCand; n < len(candidates); n++ {
			j := candidates[n]
			if sels[j].set.Bit(int(bit)) == 0 {
				continue
			}
			// 该集合必须整体落在未覆盖部分中，否则会与已选集合重叠，
			// “无重叠求和”不成立。
			if !subsetOf(sels[j].set, uncovered) {
				continue
			}
			chosen = append(chosen, j)
			next := new(big.Int).AndNot(uncovered, sels[j].set)
			if dfs(n+1, next) { // n+1：下标递增，天然排除重叠排列
				return true
			}
			chosen = chosen[:len(chosen)-1]
		}
		return false
	}

	if dfs(0, new(big.Int).Set(sels[target].set)) {
		sort.Ints(chosen)
		return chosen
	}
	return nil
}

// histogramFinding 生成“可合并为一次直方图发布”的提示，并估算可节省的 ε。
// 合并后按组内最大单笔 ε 发布即可保证每个桶的噪声不大于原先，
// 节省量 ≈ Σ ε_i − max ε_i。
func histogramFinding(queries []Query, results []Result, group []int) Finding {
	var sum, maxEps float64
	names := make([]string, len(group))
	for k, idx := range group {
		sum += results[idx].Epsilon
		if results[idx].Epsilon > maxEps {
			maxEps = results[idx].Epsilon
		}
		names[k] = fmt.Sprintf("%q", queries[idx].Name)
	}
	wasted := sum - maxEps
	return Finding{
		Kind:          kindHistogram,
		Severity:      sevMedium,
		QueryIndices:  append([]int(nil), group...),
		WastedEpsilon: wasted,
		Message: fmt.Sprintf(
			"互不相交的子集计数 %s 被拆成 %d 次独立查询，顺序组合共消耗 ε=%.4f；"+
				"它们可合并为一次直方图发布（L1 敏感度仍为 1），同等噪声尺度下仅需约 ε=%.4f。",
			strings.Join(names, "、"), len(group), sum, maxEps),
	}
}

func qNameList(queries []Query, idx []int) string {
	names := make([]string, len(idx))
	for k, i := range idx {
		names[k] = fmt.Sprintf("%q", queries[i].Name)
	}
	return strings.Join(names, " + ")
}

func subsetOf(a, b *big.Int) bool {
	return new(big.Int).And(a, b).Cmp(a) == 0
}

func disjoint(a, b *big.Int) bool {
	return new(big.Int).And(a, b).Sign() == 0
}

func equalSet(a, b *big.Int) bool {
	return a.Cmp(b) == 0
}

// lowestSetBit 返回 x 中最低置位的下标（x 必须非零）。
func lowestSetBit(x *big.Int) uint {
	for i, w := range x.Bits() {
		if w != 0 {
			return uint(i)*uint(bits.UintSize) + uint(bits.TrailingZeros(uint(w)))
		}
	}
	return 0 // 仅在 x=0 时发生，调用处保证 x 非零
}
