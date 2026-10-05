package dp

import (
	"math"
	"math/big"
	"testing"
)

// buildSelections 用记录下标构造命中位集，便于精确测试组合模式分析。
func buildSelections(seqs ...[]int) []selection {
	out := make([]selection, len(seqs))
	for i, seq := range seqs {
		s := selection{set: newBigSet(seq...)}
		out[i] = s
	}
	return out
}

func newBigSet(idxs ...int) *big.Int {
	x := new(big.Int)
	for _, i := range idxs {
		x.SetBit(x, i, 1)
	}
	return x
}

func acceptedResults(eps ...float64) []Result {
	rs := make([]Result, len(eps))
	for i, e := range eps {
		rs[i] = Result{Index: i, Name: "q" + string(rune('a'+i)),
			Epsilon: e, Accepted: true}
	}
	return rs
}

func queriesNamed(names ...string) []Query {
	qs := make([]Query, len(names))
	for i, n := range names {
		qs[i] = Query{Name: n}
	}
	return qs
}

func TestAnalyzeDuplicate(t *testing.T) {
	queries := queriesNamed("first", "other", "again")
	results := acceptedResults(0.2, 0.1, 0.2)
	sels := buildSelections(
		[]int{0, 1, 2},
		[]int{3, 4},
		[]int{2, 1, 0}, // 与第一个集合相同（顺序无关）
	)
	f := Analyze(queries, results, sels)
	var dup *Finding
	for i := range f {
		if f[i].Kind == kindDuplicate {
			dup = &f[i]
		}
	}
	if dup == nil {
		t.Fatalf("应识别重复查询, got %+v", f)
	}
	if !sameIndices(dup.QueryIndices, []int{0, 2}) {
		t.Errorf("重复对下标 = %v, want [0 2]", dup.QueryIndices)
	}
	if dup.WastedEpsilon != 0.2 {
		t.Errorf("浪费 ε = %v, want 0.2", dup.WastedEpsilon)
	}
}

func TestAnalyzeDerivable(t *testing.T) {
	// 先查总数（集合 0..5），再拆成两个不相交子集；之后又问总数可由两子集推出。
	queries := queriesNamed("total", "part_a", "part_b", "total_again")
	results := acceptedResults(0.3, 0.2, 0.2, 0.3)
	sels := buildSelections(
		[]int{0, 1, 2, 3, 4, 5},
		[]int{0, 1, 2},
		[]int{3, 4, 5},
		[]int{0, 1, 2, 3, 4, 5},
	)
	f := Analyze(queries, results, sels)
	var der *Finding
	for i := range f {
		if f[i].Kind == kindDerivable {
			der = &f[i]
		}
	}
	if der == nil {
		t.Fatalf("应识别可由子集后处理推导的查询, got %+v", f)
	}
	if !sameIndices(der.QueryIndices, []int{1, 2, 3}) {
		t.Errorf("推导链下标 = %v, want [1 2 3]", der.QueryIndices)
	}
	if der.WastedEpsilon != 0.3 {
		t.Errorf("浪费 ε = %v, want 0.3", der.WastedEpsilon)
	}
}

func TestAnalyzeDerivableExactCover(t *testing.T) {
	// 存在互相重叠的候选集合，但唯一精确划分是 {a,c}：
	// target={0,1,2,3}, x={0,1}, y={1,2}, z={2,3}。
	// 贪心装入 x 后会被迫选 z，留下 2 号位无法覆盖；DFS 必须找到 x? 不——
	// x+z 缺位 2 吗？x={0,1}, z={2,3} 恰好是精确划分。y 与二者都重叠。
	queries := queriesNamed("x", "y", "z", "total")
	results := acceptedResults(0.1, 0.1, 0.1, 0.3)
	sels := buildSelections(
		[]int{0, 1},
		[]int{1, 2},
		[]int{2, 3},
		[]int{0, 1, 2, 3},
	)
	f := Analyze(queries, results, sels)
	var der *Finding
	for i := range f {
		if f[i].Kind == kindDerivable {
			der = &f[i]
		}
	}
	if der == nil {
		t.Fatalf("重叠候选存在时也应找到精确划分 {x,z}, got %+v", f)
	}
	if !sameIndices(der.QueryIndices, []int{0, 2, 3}) {
		t.Errorf("精确划分下标 = %v, want [0 2 3]", der.QueryIndices)
	}
}

func TestAnalyzeNotDerivableWhenOverlapLeftover(t *testing.T) {
	// target={0,1,2,3}，候选 {0,1} 与 {1,2,3} 重叠，无法无重叠求和。
	queries := queriesNamed("a", "b", "total")
	results := acceptedResults(0.2, 0.2, 0.3)
	sels := buildSelections(
		[]int{0, 1},
		[]int{1, 2, 3},
		[]int{0, 1, 2, 3},
	)
	for _, x := range Analyze(queries, results, sels) {
		if x.Kind == kindDerivable {
			t.Errorf("存在重叠且无法覆盖时不应判为可推导: %+v", x)
		}
	}
}

func TestAnalyzeHistogramOpportunity(t *testing.T) {
	queries := queriesNamed("c1", "c2", "c3", "c4")
	results := acceptedResults(0.1, 0.1, 0.1, 0.1)
	sels := buildSelections(
		[]int{0, 1}, []int{2, 3}, []int{4, 5}, []int{6, 7},
	)
	f := Analyze(queries, results, sels)
	var hist *Finding
	for i := range f {
		if f[i].Kind == kindHistogram {
			hist = &f[i]
		}
	}
	if hist == nil {
		t.Fatalf("4 个互不相交子集应提示合并直方图, got %+v", f)
	}
	if !sameIndices(hist.QueryIndices, []int{0, 1, 2, 3}) {
		t.Errorf("直方图组成员 = %v", hist.QueryIndices)
	}
	if math.Abs(hist.WastedEpsilon-0.3) > 1e-12 { // Σ0.4 − max0.1
		t.Errorf("可节省 ε = %v, want 0.3", hist.WastedEpsilon)
	}
}

func TestAnalyzeIgnoresRejectedAndEmpty(t *testing.T) {
	queries := queriesNamed("a", "rejected", "b")
	results := []Result{
		{Index: 0, Name: "a", Epsilon: 0.1, Accepted: true},
		{Index: 1, Name: "rejected", Epsilon: 0.9, Accepted: false},
		{Index: 2, Name: "b", Epsilon: 0.1, Accepted: true},
	}
	sels := buildSelections([]int{0, 1}, nil, []int{2, 3})
	// 被拒绝查询的位集为 nil，分析不得解引用，也不应产出任何模式。
	f := Analyze(queries, results, sels)
	if len(f) != 0 {
		t.Errorf("两个不相关的已接受查询不应产生模式, got %+v", f)
	}
}

func TestAnalyzeEmpty(t *testing.T) {
	if got := Analyze(nil, nil, nil); len(got) != 0 {
		t.Errorf("空输入应返回空（非 nil 也行，但不得有发现）, got %+v", got)
	}
}

func sameIndices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
