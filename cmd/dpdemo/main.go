// Command dpdemo 运行一组演示性的差分隐私计数查询，生成单一静态报告
// report.html（当前目录），用浏览器直接打开即可，无需启动任何服务。
package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"

	dp "github.com/TommyMarsss/differential-privacy"
)

func main() {
	out := flag.String("o", "report.html", "静态报告输出路径")
	seed := flag.Uint64("seed", 20261005, "Laplace 噪声随机种子（固定种子可复现报告）")
	flag.Parse()

	ds := buildDataset()
	queries := buildQueries()

	engine, err := dp.NewEngine(1.0, rand.New(rand.NewPCG(*seed, 0x9e3779b97f4a7c15)))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	rep := engine.Run("Laplace 机制计数查询 · 隐私预算报告", ds, queries)

	if err := dp.WriteHTML(*out, rep); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("预算上限 ε=%.4f，已发布 %d 次、拒绝 %d 次，累计消耗 ε=%.6f\n",
		rep.BudgetTotal, rep.Accepted, rep.Rejected, rep.BudgetUsed)
	for _, r := range rep.Results {
		if r.Accepted {
			fmt.Printf("  [发布] #%d %-20s 真实=%-5d 发布=%-5d 剩余 ε=%.6f\n",
				r.Index, r.Name, r.TrueValue, r.PublishedValue, r.RemainingAfter)
		} else {
			fmt.Printf("  [拒绝] #%d %-20s %s\n", r.Index, r.Name, r.RejectReason)
		}
	}
	fmt.Printf("识别出 %d 种低效查询模式；报告已写入 %s\n", len(rep.Findings), *out)
}

// buildDataset 构造 2000 条确定性演示记录：4 个城市（700/600/450/250）、
// 3 个年龄段（800/600/600），全部为硬编码计数，避免随机性掩盖查询语义。
func buildDataset() *dp.Dataset {
	cities := []struct {
		name string
		n    int
	}{
		{"上海", 700}, {"北京", 600}, {"深圳", 450}, {"杭州", 250},
	}
	ages := []struct {
		name string
		n    int
	}{
		{"20-29", 800}, {"30-39", 600}, {"40-49", 600},
	}

	records := make([]dp.Record, 0, 2000)
	i := 0
	for _, c := range cities {
		for range c.n {
			records = append(records, dp.Record{"city": c.name})
			i++
		}
	}
	// 按记录下标赋年龄段（与城市正交），精确控制各年龄段人数。
	for idx := range records {
		switch {
		case idx < ages[0].n:
			records[idx]["age"] = ages[0].name
		case idx < ages[0].n+ages[1].n:
			records[idx]["age"] = ages[1].name
		default:
			records[idx]["age"] = ages[2].name
		}
	}
	return &dp.Dataset{Name: "演示用户数据集（城市 × 年龄段）", Records: records}
}

func eq(attr, val string) func(dp.Record) bool {
	return func(r dp.Record) bool { return r[attr] == val }
}

func all() func(dp.Record) bool {
	return func(dp.Record) bool { return true }
}

// buildQueries 刻意覆盖：正常直方图式拆分、重复提问（先总数后子集的
// 可推导冗余）、超预算拒绝、非法参数拒绝、极小 ε 导致噪声淹没信号。
func buildQueries() []dp.Query {
	return []dp.Query{
		{Name: "total_users", Description: "全表用户总数", Epsilon: 0.2, Predicate: all()},
		{Name: "city_shanghai", Description: "城市 = 上海", Epsilon: 0.1, Predicate: eq("city", "上海")},
		{Name: "city_beijing", Description: "城市 = 北京", Epsilon: 0.1, Predicate: eq("city", "北京")},
		{Name: "city_shenzhen", Description: "城市 = 深圳", Epsilon: 0.1, Predicate: eq("city", "深圳")},
		{Name: "city_other", Description: "城市 = 杭州", Epsilon: 0.1, Predicate: eq("city", "杭州")},
		{Name: "total_users_again", Description: "全表用户总数（重复提问，可直接复用首次答案）",
			Epsilon: 0.2, Predicate: all()},
		{Name: "age_30_39", Description: "年龄段 = 30-39（申请时预算已耗尽）",
			Epsilon: 0.3, Predicate: eq("age", "30-39")},
		{Name: "bad_param", Description: "非法参数：ε = 0",
			Epsilon: 0, Predicate: eq("age", "20-29")},
		{Name: "age_20_29_tiny", Description: "年龄段 = 20-29（用剩余极小预算；预算恰好用尽时此查询将被拒绝）",
			Epsilon: 0.000001, Predicate: eq("age", "20-29")},
	}
}
