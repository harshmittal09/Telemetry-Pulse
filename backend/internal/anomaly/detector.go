package anomaly

import (
	"telemetrypulse/internal/window"
)

type Detector struct {
	windows map[string]*window.Window
}

func NewDetector() *Detector {
	return &Detector{
		windows: make(map[string]*window.Window),
	}
}

func (d *Detector) Analyze(url string, latency float64) (float64, bool) {
	w := d.windows[url]

	if w == nil {
		d.windows[url] = window.MakeWindow(100)
		w = d.windows[url]
	}

	zScore := w.ZScore(latency)

	w.Add(latency)

	if zScore > 3 {
		return zScore, true
	}

	return zScore, false

}
