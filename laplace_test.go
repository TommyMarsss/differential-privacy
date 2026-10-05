package dp

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestLaplaceScale(t *testing.T) {
	cases := []struct {
		sens, eps, want float64
	}{
		{1, 1, 1},
		{1, 0.5, 2},
		{1, 0.1, 10},
		{2, 0.5, 4},   // 敏感度翻倍，尺度翻倍
		{3, 0.25, 12}, // Δ/ε 组合
		{0.5, 0.25, 2},
	}
	for _, c := range cases {
		got, err := LaplaceScale(c.sens, c.eps)
		if err != nil {
			t.Fatalf("LaplaceScale(%v,%v) unexpected error: %v", c.sens, c.eps, err)
		}
		if math.Abs(got-c.want) > 1e-12 {
			t.Errorf("LaplaceScale(%v,%v) = %v, want %v", c.sens, c.eps, got, c.want)
		}
	}
}

func TestLaplaceScaleInvalid(t *testing.T) {
	bad := []struct{ sens, eps float64 }{
		{0, 1}, {-1, 1}, {math.NaN(), 1}, {math.Inf(1), 1},
		{1, 0}, {1, -0.1}, {1, math.NaN()}, {1, math.Inf(-1)},
	}
	for _, c := range bad {
		if _, err := LaplaceScale(c.sens, c.eps); err == nil {
			t.Errorf("LaplaceScale(%v,%v) expected error, got nil", c.sens, c.eps)
		}
	}
}

// TestSampleLaplaceStatistics 验证 Laplace 采样器的统计性质：
// 大样本下均值应趋于理论均值 0，样本方差应趋于理论方差 2b²。
// 阈值取约 4~5 倍标准误，防止 CI 上的随机抖动造成误报。
func TestSampleLaplaceStatistics(t *testing.T) {
	const n = 200000
	rng := rand.New(rand.NewPCG(0x1234abcd, 0x5678ef01))

	for _, b := range []float64{0.5, 2.0, 5.0} {
		samples := make([]float64, n)
		var sum float64
		for i := range samples {
			x := SampleLaplace(rng, b)
			if math.IsNaN(x) || math.IsInf(x, 0) {
				t.Fatalf("b=%v: sample is not finite: %v", b, x)
			}
			samples[i] = x
			sum += x
		}
		mean := sum / n
		// 均值标准误 sqrt(Var/n) = b·sqrt(2/n)。
		meanSE := b * math.Sqrt(2.0/n)
		if math.Abs(mean) > 5*meanSE {
			t.Errorf("b=%v: sample mean = %.5f, want within %.5f of 0 (理论 E[X]=0)",
				b, mean, 5*meanSE)
		}

		var sq float64
		for _, x := range samples {
			d := x - mean
			sq += d * d
		}
		variance := sq / (n - 1)
		wantVar := 2.0 * b * b
		// Laplace 四阶矩 μ4=24b⁴，方差估计量的标准误约为
		// sqrt((μ4-σ⁴)/n) = sqrt(20b⁴/n)。
		varSE := math.Sqrt(20.0) * b * b / math.Sqrt(n)
		if math.Abs(variance-wantVar) > 5*varSE {
			t.Errorf("b=%v: sample variance = %.5f, want within %.5f of %.5f (理论 Var=2b²)",
				b, variance, 5*varSE, wantVar)
		}
		t.Logf("b=%.1f: mean=%.5f (SE≈%.5f), variance=%.5f, 理论 2b²=%.3f (SE≈%.5f)",
			b, mean, meanSE, variance, wantVar, varSE)
	}
}

// TestSampleLaplaceShape 用中位数与生存函数做分布形状的补充校验：
// Laplace 关于 0 对称，且 P(|X| > t) = exp(-t/b)。
func TestSampleLaplaceShape(t *testing.T) {
	const n = 200000
	rng := rand.New(rand.NewPCG(0xdeadbeef, 1))
	b := 3.0

	neg, pos := 0, 0
	threshold := b * 2.0 // t=2b，P(|X|>t)=e^-2 ≈ 0.1353
	exceed := 0
	for range n {
		x := SampleLaplace(rng, b)
		if x < 0 {
			neg++
		} else {
			pos++
		}
		if math.Abs(x) > threshold {
			exceed++
		}
	}
	if frac := float64(neg) / n; math.Abs(frac-0.5) > 0.01 {
		t.Errorf("负样本占比 = %.4f, want ≈0.5（分布关于 0 对称）", frac)
	}
	got := float64(exceed) / n
	want := math.Exp(-threshold / b)
	if math.Abs(got-want) > 0.005 {
		t.Errorf("P(|X|>2b) 经验值 = %.4f, want ≈%.4f", got, want)
	}
}
