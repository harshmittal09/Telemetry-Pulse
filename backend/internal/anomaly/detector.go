package anomaly

import (
	"telemetrypulse/internal/window"
)

const (
	windowSize = 100
	minSamples = 20   // don't call anything an anomaly until we have a real baseline
	zThreshold = 3.0  // how many stddevs above normal counts as a spike
	minJumpMs  = 20.0 // a "3 sigma" jump that is only a few ms isn't worth an alert
)

type Result struct {
	ZScore   float64
	Baseline float64 // average latency of the window before this reading
	Anomaly  bool
}

type Detector struct {
	windows map[string]*window.Window
}

func NewDetector() *Detector {
	return &Detector{
		windows: make(map[string]*window.Window),
	}
}

// Analyze should only be called with healthy readings, failed requests
// would drag the baseline down because they come back almost instantly.
func (d *Detector) Analyze(url string, latency float64) Result {
	w := d.windows[url]

	if w == nil {
		w = window.MakeWindow(windowSize)
		d.windows[url] = w
	}

	mean := w.Mean()
	stddev := w.StandardDev()

	res := Result{
		ZScore:   w.ZScore(latency),
		Baseline: mean,
	}
	res.Anomaly = w.Count() >= minSamples && res.ZScore > zThreshold && latency-mean > minJumpMs

	if res.Anomaly {
		// add a capped value instead of the raw spike, otherwise one 5s timeout
		// blows up the stddev and hides every problem after it
		w.Add(mean + zThreshold*stddev)
	} else {
		w.Add(latency)
	}

	return res
}
