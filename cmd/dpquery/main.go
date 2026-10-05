// dpquery 演示：在合成员工数据集上执行一组计数查询，
// 注入 Laplace 噪声、记账隐私预算，并生成单一静态 HTML 报告 report.html。
package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"time"

	"github.com/TommyMarsss/differential-privacy/dp"
)

// makeDataset 生成确定性的合成员工数据集（部门 + 年龄段）。
func makeDataset(n int) dp.Dataset {
	rng := rand.New(rand.NewPCG(42, 7))
	depts := []string{"Engineering", "Sales", "HR", "Support"}
	weights := []float64{0.45, 0.25, 0.10, 0.20}
	records := make([]dp.Record, 0, n)
	for i := 0; i < n; i++ {
		x := rng.Float64()
		dept := depts[len(depts)-1]
		acc := 0.0
		for j, w := range weights {
			acc += w
			if x < acc {
				dept = depts[j]
				break
			}
		}
		age := 20 + rng.IntN(41) // 20–60 岁
		records = append(records, dp.Record{
			"dept": dept,
			"age":  fmt.Sprintf("%d", age),
		})
	}
	return dp.Dataset{Name: "employees（合成数据）", Records: records}
}

func main() {
	out := flag.String("o", "report.html", "输出 HTML 报告路径")
	budget := flag.Float64("budget", 4.0, "隐私预算上限 ε")
	n := flag.Int("n", 1000, "数据集记录数")
	flag.Parse()

	data := makeDataset(*n)
	acct, err := dp.NewAccountant(*budget)
	if err != nil {
		log.Fatal(err)
	}
	// 演示使用固定种子的随机源以便复现；生产环境传 nil 使用 crypto/rand。
	src := rand.New(rand.NewPCG(2024, 11))
	engine := dp.NewEngine(data, acct, src)

	count := func(pred func(dp.Record) bool) func(dp.Record) bool { return pred }
	isDept := func(d string) func(dp.Record) bool {
		return func(r dp.Record) bool { return r["dept"] == d }
	}
	ageAtLeast := func(a int) func(dp.Record) bool {
		return func(r dp.Record) bool {
			var v int
			fmt.Sscanf(r["age"], "%d", &v)
			return v >= a
		}
	}

	queries := []dp.Query{
		{ID: "Q1", Description: "员工总数", Epsilon: 0.5,
			Predicate: count(func(dp.Record) bool { return true }), Key: "count:*"},
		{ID: "Q2", Description: "Engineering 部门人数", Epsilon: 0.5,
			Predicate: isDept("Engineering"), Key: "count:dept=Engineering", RefinesKey: "count:*"},
		{ID: "Q3", Description: "Sales 部门人数", Epsilon: 0.5,
			Predicate: isDept("Sales"), Key: "count:dept=Sales", RefinesKey: "count:*"},
		{ID: "Q4", Description: "HR 部门人数", Epsilon: 0.5,
			Predicate: isDept("HR"), Key: "count:dept=HR", RefinesKey: "count:*"},
		{ID: "Q5", Description: "Support 部门人数", Epsilon: 0.5,
			Predicate: isDept("Support"), Key: "count:dept=Support", RefinesKey: "count:*"},
		{ID: "Q6", Description: "员工总数（重复查询）", Epsilon: 0.5,
			Predicate: count(func(dp.Record) bool { return true }), Key: "count:*"},
		{ID: "Q7", Description: "年龄 ≥ 40 的员工数", Epsilon: 1.0,
			Predicate: ageAtLeast(40), Key: "count:age>=40"},
		{ID: "Q8", Description: "年龄 ≥ 30 的员工数（将超预算）", Epsilon: 1.5,
			Predicate: ageAtLeast(30), Key: "count:age>=30"},
		{ID: "Q9", Description: "Engineering 且年龄 ≥ 40 的人数", Epsilon: 0.5,
			Predicate: func(r dp.Record) bool {
				return isDept("Engineering")(r) && ageAtLeast(40)(r)
			}, Key: "count:dept=Engineering,age>=40", RefinesKey: "count:dept=Engineering"},
	}

	results := engine.Run(queries)
	for _, r := range results {
		fmt.Println(r)
	}

	findings := dp.AnalyzeComposition(queries)
	for _, f := range findings {
		fmt.Printf("[组合分析·%s] %s\n", f.Kind, f.Detail)
	}

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	err = dp.WriteReport(f, dp.ReportData{
		Title:     "差分隐私计数查询报告（Laplace 机制）",
		Dataset:   data.Name,
		RecordN:   len(data.Records),
		Generated: time.Now(),
		Summary:   dp.Summarize(results, acct),
		Results:   results,
		Findings:  findings,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\n报告已生成：%s（预算 %.4g，已用 %.4g，剩余 %.4g）\n",
		*out, acct.Budget(), acct.Spent(), acct.Remaining())
}
