// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package utils




import (
	"math"
	"math/rand"
)

var rng = rand.New(rand.NewSource(42))

type LinearRegression struct {
	Slope     float64
	Intercept float64
	RSquared  float64
}

func FitLinearRegression(x, y []float64) LinearRegression {
	n := len(x)
	if n < 2 {
		return LinearRegression{}
	}
	meanX := Mean(x)
	meanY := Mean(y)

	var num, denomX float64
	for i := 0; i < n; i++ {
		dx := x[i] - meanX
		dy := y[i] - meanY
		num += dx * dy
		denomX += dx * dx
	}

	var slope float64
	if denomX != 0 {
		slope = num / denomX
	}
	intercept := meanY - slope*meanX

	var ssRes, ssTot float64
	for i := 0; i < n; i++ {
		pred := slope*x[i] + intercept
		res := y[i] - pred
		ssRes += res * res
		dy := y[i] - meanY
		ssTot += dy * dy
	}
	var r2 float64
	if ssTot != 0 {
		r2 = 1 - ssRes/ssTot
	}
	if r2 < 0 {
		r2 = 0
	}

	return LinearRegression{
		Slope:     slope,
		Intercept: intercept,
		RSquared:  r2,
	}
}

func Predict(lr LinearRegression, x float64) float64 {
	return lr.Slope*x + lr.Intercept
}

func StdError(residuals []float64) float64 {
	if len(residuals) < 2 {
		return 0
	}
	mean := Mean(residuals)
	sumSq := 0.0
	for _, r := range residuals {
		d := r - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(residuals)-1))
}

func ZScore(value float64, mean, std float64) float64 {
	if std < 1e-9 {
		if math.Abs(value-mean) > 1e-6 {
			return math.Inf(1)
		}
		return 0
	}
	return math.Abs(value-mean) / std
}

func NormalRandom(mean, std float64) float64 {
	u1 := 1.0 - rng.Float64()
	u2 := 1.0 - rng.Float64()
	z0 := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
	return mean + z0*std
}