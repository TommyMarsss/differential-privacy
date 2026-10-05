// Package dp 实现基于 Laplace 机制的纯 ε-差分隐私计数查询系统：
// 噪声注入、隐私预算记账、顺序组合（sequential composition）核算、
// 低效查询模式识别，以及单一静态 HTML 报告生成。
//
// 本包只依赖 Go 标准库，不使用任何第三方差分隐私或统计库。
package dp

import (
	"fmt"
	"math"
)

// RNG 是 Laplace 采样所需的均匀随机源。
// math/rand/v2 中的 *rand.Rand 天然满足该接口，测试时可注入固定种子的
// 随机源以获得可复现结果。
type RNG interface {
	// Float64 返回半开区间 [0,1) 上的均匀随机数。
	Float64() float64
}

// LaplaceScale 返回 Laplace 机制的噪声尺度 b = sensitivity/epsilon。
//
// sensitivity 是查询的 L1 敏感度：相邻数据集（相差一条记录）下计数结果的
// 最大改变量。单表计数（一条记录只被计数一次）敏感度为 1；同一行被多次
// 计数的向量查询需按 L1 范数另行计算。
//
// ε 必须为正的有限数；ε 越小噪声越大、隐私越强，ε→∞ 时噪声趋于 0、
// 隐私保护趋于消失，因此 ε=0 或负数属于非法参数，而不是“无限隐私”。
func LaplaceScale(sensitivity, epsilon float64) (float64, error) {
	if !(sensitivity > 0) || math.IsNaN(sensitivity) || math.IsInf(sensitivity, 0) {
		return 0, fmt.Errorf("dp: sensitivity must be a positive finite number, got %v", sensitivity)
	}
	if !(epsilon > 0) || math.IsNaN(epsilon) || math.IsInf(epsilon, 0) {
		return 0, fmt.Errorf("dp: epsilon must be a positive finite number, got %v", epsilon)
	}
	return sensitivity / epsilon, nil
}

// SampleLaplace 从位置参数为 0、尺度为 scale 的 Laplace 分布抽取一个样本。
//
// 采用逆变换采样。Laplace(0,b) 的分布函数为
//
//	F(x) = 1/2 + 1/2 · sgn(x) · (1 - exp(-|x|/b))
//
// 令 U ~ Uniform(-1/2, 1/2)，其逆函数为
//
//	X = -b · sgn(U) · ln(1 - 2|U|)
//
// 理论统计性质：E[X]=0，Var(X)=2b²。
func SampleLaplace(rng RNG, scale float64) float64 {
	u := rng.Float64() - 0.5 // U ∈ [-0.5, 0.5)
	absU := math.Abs(u)
	// Float64() 理论上可能恰好返回 0，此时 1-2|U|=0、ln(0)=-Inf。
	// 该事件概率仅 2^-53，这里做一次保守截断以保证输出始终有限。
	if absU >= 0.5 {
		absU = 0.5 - 1e-16
	}
	if u >= 0 {
		return -scale * math.Log(1-2*absU) // 正半轴
	}
	return scale * math.Log(1-2*absU) // 负半轴
}

// publish 是对加噪计数的后处理（post-processing）：四舍五入并截断到非负。
// 后处理不接触原始数据，不会削弱差分隐私保证，也不额外计费。
func publish(noisy float64) int {
	v := math.Round(noisy)
	if v < 0 {
		v = 0
	}
	return int(v)
}
