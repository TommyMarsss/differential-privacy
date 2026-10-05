// Package dp 实现基于 Laplace 机制的差分隐私计数查询：
// 噪声采样、隐私预算记账、基础序列组合与静态报告生成。
package dp

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// UniformSource 返回 [0,1) 区间均匀分布的随机数。
// 生产环境必须使用密码学安全的随机源（见 CryptoSource），
// 否则噪声可预测，差分隐私保证将失效。
type UniformSource interface {
	Float64() float64
}

// CryptoSource 基于 crypto/rand 的均匀随机源，用于真实的隐私保护场景。
type CryptoSource struct{}

// Float64 返回 [0,1) 内均匀分布的 53 位精度随机数。
func (CryptoSource) Float64() float64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(fmt.Sprintf("dp: crypto/rand 不可用: %v", err))
	}
	// 取高 53 位构造 [0,1) 均匀浮点数。
	return float64(binary.BigEndian.Uint64(buf[:])>>11) / (1 << 53)
}

// LaplaceScale 计算 Laplace 机制的噪声尺度 b = Δf / ε。
// sensitivity 为查询的 L1 敏感度（计数查询通常为 1），epsilon 必须为正。
func LaplaceScale(sensitivity, epsilon float64) (float64, error) {
	if epsilon <= 0 || math.IsNaN(epsilon) || math.IsInf(epsilon, 0) {
		return 0, fmt.Errorf("dp: 非法的隐私参数 ε=%v（必须为正有限值）", epsilon)
	}
	if sensitivity <= 0 || math.IsNaN(sensitivity) || math.IsInf(sensitivity, 0) {
		return 0, fmt.Errorf("dp: 非法的敏感度 Δf=%v（必须为正有限值）", sensitivity)
	}
	return sensitivity / epsilon, nil
}

// LaplaceVariance 返回尺度为 b 的 Laplace 分布的理论方差 2b²。
func LaplaceVariance(b float64) float64 { return 2 * b * b }

// Sampler 从 Laplace 分布采样噪声。
type Sampler struct {
	src UniformSource
}

// NewSampler 用给定的均匀随机源构造采样器。
func NewSampler(src UniformSource) *Sampler {
	if src == nil {
		src = CryptoSource{}
	}
	return &Sampler{src: src}
}

// Laplace 采样一个均值为 0、尺度为 b 的 Laplace 随机变量，
// 使用逆变换采样：X = -b · sign(U) · ln(1 - 2|U|)，U ~ Uniform(-1/2, 1/2)。
func (s *Sampler) Laplace(b float64) float64 {
	u := s.src.Float64() - 0.5
	if u == 0 {
		return 0
	}
	sign := 1.0
	if u < 0 {
		sign = -1
	}
	return -b * sign * math.Log1p(-2*math.Abs(u))
}

// ErrBudgetExhausted 在隐私预算不足时由查询引擎返回（包装在 QueryError 中）。
var ErrBudgetExhausted = errors.New("dp: 隐私预算不足，查询被拒绝")
