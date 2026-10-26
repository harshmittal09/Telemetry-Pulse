package window

import (
	"math"
	"math/rand"
	"testing"
)

func naiveStats(vals []float64) (float64, float64) {
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	mean := sum / float64(len(vals))
	sq := 0.0
	for _, v := range vals {
		sq += (v - mean) * (v - mean)
	}
	return mean, math.Sqrt(sq / float64(len(vals)))
}

func TestWindowMatchesNaiveAfterWrapping(t *testing.T) {
	w := MakeWindow(50)
	var last []float64
	r := rand.New(rand.NewSource(1))

	for i := 0; i < 5000; i++ {
		v := 30 + r.ExpFloat64()*20
		w.Add(v)
		last = append(last, v)
		if len(last) > 50 {
			last = last[1:]
		}
	}

	mean, sd := naiveStats(last)
	if math.Abs(w.Mean()-mean) > 1e-9 || math.Abs(w.StandardDev()-sd) > 1e-9 {
		t.Fatalf("got mean=%v sd=%v, want mean=%v sd=%v", w.Mean(), w.StandardDev(), mean, sd)
	}
	if w.Count() != 50 {
		t.Fatalf("count = %d, want 50", w.Count())
	}
}

func TestZeroValuesAreCountedToo(t *testing.T) {
	w := MakeWindow(3)
	for _, v := range []float64{0, 0, 9} {
		w.Add(v)
	}
	if w.Mean() != 3 {
		t.Fatalf("mean = %v, want 3", w.Mean())
	}
}

func TestZScoreIsZeroWithNoSpread(t *testing.T) {
	w := MakeWindow(10)
	w.Add(40)
	w.Add(40)
	if z := w.ZScore(500); z != 0 {
		t.Fatalf("z = %v, want 0 when stddev is 0", z)
	}
}
