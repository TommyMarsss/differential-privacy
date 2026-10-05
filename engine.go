package dp

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// Record 是数据集中的一条记录，用属性名到属性值的映射表示。
type Record map[string]string

// Dataset 是命名的记录集合。相邻数据集定义为相差恰好一条记录，
// 因此“某谓词命中多少条记录”这类单表计数的 L1 敏感度恒为 1。
type Dataset struct {
	Name    string
	Records []Record
}

// Size 返回记录条数。
func (d *Dataset) Size() int { return len(d.Records) }

// Query 描述一次计数查询：在数据集上统计满足 Predicate 的记录数，
// 申请 epsilon 的隐私预算，并以给定 L1 敏感度运行 Laplace 机制。
//
// Sensitivity 留空（0）时按单表计数取 1。
type Query struct {
	Name        string
	Description string
	Epsilon     float64
	Sensitivity float64
	Predicate   func(Record) bool
}

// Result 是一次查询请求的完整处理记录，字段同时用于预算审计与 HTML 报告。
type Result struct {
	Index          int     `json:"index"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Epsilon        float64 `json:"epsilon"`
	Sensitivity    float64 `json:"sensitivity"`
	Scale          float64 `json:"scale"`
	TrueValue      int     `json:"trueValue"`
	Noise          float64 `json:"noise"`
	NoisyValue     float64 `json:"noisyValue"`
	PublishedValue int     `json:"publishedValue"`
	Accepted       bool    `json:"accepted"`
	RejectReason   string  `json:"rejectReason,omitempty"`
	UsedBefore     float64 `json:"usedBefore"`
	UsedAfter      float64 `json:"usedAfter"`
	RemainingAfter float64 `json:"remainingAfter"`
}

// Report 是一组查询处理结束后的完整快照，可直接序列化为报告数据。
type Report struct {
	Title       string    `json:"title"`
	GeneratedAt string    `json:"generatedAt"`
	DatasetName string    `json:"datasetName"`
	DatasetSize int       `json:"datasetSize"`
	BudgetTotal float64   `json:"budgetTotal"`
	BudgetUsed  float64   `json:"budgetUsed"`
	Accepted    int       `json:"accepted"`
	Rejected    int       `json:"rejected"`
	Results     []Result  `json:"results"`
	Findings    []Finding `json:"findings"`
}

// Engine 按提交顺序执行查询：先校验与预算检查，通过后才记账并注入噪声，
// 全部查询处理完后统一进行低效模式分析。
type Engine struct {
	acc *Accountant
	rng RNG
}

// NewEngine 创建预算上限为 total、噪声随机源为 rng 的执行引擎。
// rng 为 nil 时 panic 属编程错误，故要求调用方显式提供（可注入固定种子）。
func NewEngine(total float64, rng RNG) (*Engine, error) {
	acc, err := NewAccountant(total)
	if err != nil {
		return nil, err
	}
	if rng == nil {
		return nil, fmt.Errorf("dp: nil random source")
	}
	return &Engine{acc: acc, rng: rng}, nil
}

// selection 记录一次已接受查询命中的记录集合（位图），供组合模式分析使用。
type selection struct {
	set *big.Int // 命中记录下标构成的位集；被拒绝查询为 nil
}

// Run 依次处理 queries，返回包含逐笔明细与低效模式分析的报告。
// 任何单笔查询被拒绝都不会中断后续查询——拒绝本身也是一种处理结果。
func (e *Engine) Run(title string, ds *Dataset, queries []Query) *Report {
	results := make([]Result, 0, len(queries))
	sels := make([]selection, len(queries))

	for i, q := range queries {
		sens := q.Sensitivity
		if sens == 0 {
			sens = 1
		}
		r := Result{
			Index:       i,
			Name:        q.Name,
			Description: q.Description,
			Epsilon:     q.Epsilon,
			Sensitivity: sens,
		}

		// 真实值与命中集合无论查询是否被批准都计算，仅用于报告对照与
		// 离线的查询模式分析；它们不会随加噪结果对外发布。
		set := new(big.Int)
		trueCount := 0
		if q.Predicate != nil {
			for idx, rec := range ds.Records {
				if q.Predicate(rec) {
					trueCount++
					set.SetBit(set, idx, 1)
				}
			}
		}
		r.TrueValue = trueCount

		r.UsedBefore = e.acc.Used()

		// 参数校验先于预算检查：ε=0 等非法请求给出明确的参数错误，
		// 而不是含糊地报“预算不足”。
		scale, errScale := LaplaceScale(sens, q.Epsilon)
		approved, budgetReason := e.acc.Check(q.Epsilon)
		switch {
		case errScale != nil:
			r.RejectReason = "dp: query " + strconv.Quote(q.Name) + " rejected: " +
				strings.TrimPrefix(errScale.Error(), "dp: ")
		case !approved:
			// 超预算：不记账、不抽样、不发布，Scale/Noise 等保持零值。
			r.RejectReason = "dp: query " + strconv.Quote(q.Name) +
				" rejected: budget exceeded: " + budgetReason
		default:
			if err := e.acc.Spend(q.Epsilon); err != nil {
				// 理论上不可达（Check 刚通过），防御性处理：拒绝且不发布。
				r.RejectReason = "dp: query " + strconv.Quote(q.Name) +
					" rejected: " + err.Error()
				break
			}
			r.Scale = scale
			r.Noise = SampleLaplace(e.rng, scale)
			r.NoisyValue = float64(trueCount) + r.Noise
			r.PublishedValue = publish(r.NoisyValue)
			r.Accepted = true
			sels[i].set = set
		}

		r.UsedAfter = e.acc.Used()
		r.RemainingAfter = e.acc.Remaining()
		results = append(results, r)
	}

	rep := &Report{
		Title:       title,
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
		DatasetName: ds.Name,
		DatasetSize: ds.Size(),
		BudgetTotal: e.acc.Total(),
		BudgetUsed:  e.acc.Used(),
		Results:     results,
		Findings:    Analyze(queries, results, sels),
	}
	for _, r := range results {
		if r.Accepted {
			rep.Accepted++
		} else {
			rep.Rejected++
		}
	}
	return rep
}
