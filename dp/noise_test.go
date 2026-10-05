package dp

import (
	"math"
	"math/rand/v2"
	"testing"
)

// TestLaplaceScale 验证不同敏感度与 ε 组合下噪声尺度 b = Δf/ε 的计算。
func TestLaplaceScale(t *testing.T) {
	cases := []struct {
		sensitivity, epsilon, want float64
	}{
		{1, 1, 1},
		{1, 0.5, 2},
		{1, 0.1, 10},
		{2, 0.5, 4},
		{3, 2, 1.5},
		{0.5, 0.25, 2},
	}
	for _, c := range cases {
		got, err := LaplaceScale(c.sensitivity, c.epsilon)
		if err != nil {
			t.Fatalf("LaplaceScale(%v, %v) 返回错误: %v", c.sensitivity, c.epsilon, err)
		}
		if math.Abs(got-c.want) > 1e-12 {
			t.Errorf("LaplaceScale(%v, %v) = %v，期望 %v", c.sensitivity, c.epsilon, got, c.want)
		}
	}
}

// TestLaplaceScaleInvalid 验证非法参数被拒绝（防止尺度错误导致保护不足）。
func TestLaplaceScaleInvalid(t *testing.T) {
	for _, c := range [][2]float64{
		{1, 0}, {1, -1}, {0, 1}, {-2, 1},
		{math.NaN(), 1}, {1, math.NaN()},
		{math.Inf(1), 1}, {1, math.Inf(1)},
	} {
		if _, err := LaplaceScale(c[0], c[1]); err == nil {
			t.Errorf("LaplaceScale(%v, %v) 应返回错误", c[0], c[1])
		}
	}
}

// TestLaplaceStatistics 用大量样本验证噪声的统计性质：
// 均值趋近 0，方差符合理论值 2b²。
func TestLaplaceStatistics(t *testing.T) {
	const n = 500_000
	for _, b := range []float64{0.5, 1, 2, 10} {
		s := NewSampler(rand.New(rand.NewPCG(uint64(b*1000), 99)))
		var sum, sumSq float64
		for i := 0; i < n; i++ {
			x := s.Laplace(b)
			sum += x
			sumSq += x * x
		}
		mean := sum / n
		variance := sumSq/n - mean*mean
		wantVar := LaplaceVariance(b)

		// 样本均值的标准误 ≈ sqrt(2b²/n)，取 6σ 作为容差。
		meanTol := 6 * math.Sqrt(wantVar/n)
		if math.Abs(mean) > meanTol {
			t.Errorf("b=%v: 均值 %v 超出容差 ±%v（应趋近 0）", b, mean, meanTol)
		}
		// 方差估计的相对误差随 1/sqrt(n) 收敛，取 2% 容差（远大于统计涨落）。
		if rel := math.Abs(variance-wantVar) / wantVar; rel > 0.02 {
			t.Errorf("b=%v: 方差 %v 与理论值 %v 偏差 %.2f%%，超过 2%%",
				b, variance, wantVar, rel*100)
		}
	}
}

// TestLaplaceSymmetric 验证分布关于 0 对称（正负样本比例均衡）。
func TestLaplaceSymmetric(t *testing.T) {
	const n = 200_000
	s := NewSampler(rand.New(rand.NewPCG(1, 2)))
	pos := 0
	for i := 0; i < n; i++ {
		if s.Laplace(3) > 0 {
			pos++
		}
	}
	frac := float64(pos) / n
	if math.Abs(frac-0.5) > 0.01 {
		t.Errorf("正样本比例 %v，偏离 0.5 过多", frac)
	}
}
