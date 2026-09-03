package window

import "math"

type Window struct {
	capacity   int
	data       []float64
	head       int
	count      int
	sum        float64
	sumSquares float64
}

func MakeWindow(capacity int) *Window {
	return &Window{
		capacity: capacity,
		data:     make([]float64, capacity),
	}

}

func (w *Window) Add(val float64) {
	if w.data[w.head] != 0 {
		w.sum -= w.data[w.head]
		w.sumSquares -= w.data[w.head] * w.data[w.head]

	}

	w.data[w.head] = val
	w.sum += val
	w.sumSquares += val * val
	w.head++

	if w.head == w.capacity {
		w.head = 0
	}

	if w.count < w.capacity {
		w.count++
	}

}

func (w *Window) Mean() float64 {
	if w.count == 0 {
		return 0
	}
	return w.sum / float64(w.count)
}

func (w *Window) StandardDev() float64 {
	if w.count == 0 {
		return 0
	}

	variance := w.sumSquares/float64(w.count) - w.Mean()*w.Mean()

	if variance < 0 {
		variance = 0
	}

	return math.Sqrt(variance)
}

func (w *Window) ZScore(val float64) float64 {
	mean := w.Mean()
	stddev := w.StandardDev()

	if stddev == 0 {
		return 0.0
	}

	return (val - mean) / stddev

}
