package dp

import "fmt"

// Record 是数据集中的一条记录，键为属性名。
type Record map[string]string

// Dataset 是受保护的数据集。
type Dataset struct {
	Name    string
	Records []Record
}

// Query 描述一次计数查询。
type Query struct {
	ID          string            // 展示用编号，如 "Q1"
	Description string            // 人类可读的查询描述
	Epsilon     float64           // 本次查询消耗的隐私预算
	Sensitivity float64           // L1 敏感度；计数查询为 1，0 表示取默认值 1
	Predicate   func(Record) bool // 计数条件：为 true 的记录被计入
	Key         string            // 语义键（如 "count:dept=eng"），用于组合分析识别重复/包含关系
	RefinesKey  string            // 若非空，表示本查询是 Key 为该值的查询的细分子集
}

// EffectiveSensitivity 返回查询实际使用的敏感度（未设置时为计数查询默认值 1）。
func (q Query) EffectiveSensitivity() float64 {
	if q.Sensitivity == 0 {
		return 1
	}
	return q.Sensitivity
}

// Result 是一次查询的执行结果。
type Result struct {
	Query          Query
	TrueCount      int     // 真实计数值（仅报告展示用，实际系统中不应直接公开）
	NoisyCount     float64 // 加噪后的发布结果
	Noise          float64 // 注入的噪声
	Scale          float64 // 噪声尺度 b = Δf/ε
	Rejected       bool
	RejectReason   string
	SpentAfter     float64 // 本次查询后的累计消耗
	RemainingAfter float64 // 本次查询后的剩余预算
}

// Engine 在给定数据集上执行查询并记账。
type Engine struct {
	data    Dataset
	acct    *Accountant
	sampler *Sampler
}

// NewEngine 构造查询引擎。src 为 nil 时使用密码学安全随机源。
func NewEngine(data Dataset, acct *Accountant, src UniformSource) *Engine {
	return &Engine{data: data, acct: acct, sampler: NewSampler(src)}
}

// Accountant 返回引擎使用的记账器。
func (e *Engine) Accountant() *Accountant { return e.acct }

// Execute 执行一次计数查询：
// 1. 计算真实计数值；2. 校验预算，不足则拒绝（不消耗预算）；
// 3. 按 b = Δf/ε 注入 Laplace 噪声并记账。
func (e *Engine) Execute(q Query) Result {
	res := Result{Query: q}

	trueCount := 0
	for _, r := range e.data.Records {
		if q.Predicate(r) {
			trueCount++
		}
	}
	res.TrueCount = trueCount

	scale, err := LaplaceScale(q.EffectiveSensitivity(), q.Epsilon)
	if err != nil {
		res.Rejected = true
		res.RejectReason = err.Error()
		res.SpentAfter = e.acct.Spent()
		res.RemainingAfter = e.acct.Remaining()
		return res
	}
	res.Scale = scale

	if err := e.acct.Spend(q.Epsilon); err != nil {
		res.Rejected = true
		res.RejectReason = err.Error()
		res.SpentAfter = e.acct.Spent()
		res.RemainingAfter = e.acct.Remaining()
		return res
	}

	res.Noise = e.sampler.Laplace(scale)
	res.NoisyCount = float64(trueCount) + res.Noise
	res.SpentAfter = e.acct.Spent()
	res.RemainingAfter = e.acct.Remaining()
	return res
}

// Run 依次执行一组查询，返回全部结果（含被拒绝的）。
func (e *Engine) Run(queries []Query) []Result {
	results := make([]Result, 0, len(queries))
	for _, q := range queries {
		results = append(results, e.Execute(q))
	}
	return results
}

// TotalEpsilon 按基础序列组合定理计算一组已接受查询的总隐私损失：ε_total = Σ ε_i。
func TotalEpsilon(results []Result) float64 {
	total := 0.0
	for _, r := range results {
		if !r.Rejected {
			total += r.Query.Epsilon
		}
	}
	return total
}

// Summary 汇总一次运行，用于报告展示。
type Summary struct {
	Total, Accepted, Rejected int
	Budget, Spent, Remaining  float64
}

// Summarize 统计执行结果与预算状态。
func Summarize(results []Result, acct *Accountant) Summary {
	s := Summary{
		Total:     len(results),
		Budget:    acct.Budget(),
		Spent:     acct.Spent(),
		Remaining: acct.Remaining(),
	}
	for _, r := range results {
		if r.Rejected {
			s.Rejected++
		} else {
			s.Accepted++
		}
	}
	return s
}

// String 便于日志输出。
func (r Result) String() string {
	if r.Rejected {
		return fmt.Sprintf("%s 拒绝（%s）", r.Query.ID, r.RejectReason)
	}
	return fmt.Sprintf("%s 真实值=%d 加噪=%.4f (b=%.4g, 剩余预算=%.4g)",
		r.Query.ID, r.TrueCount, r.NoisyCount, r.Scale, r.RemainingAfter)
}
