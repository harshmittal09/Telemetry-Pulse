package anomaly

import (
	"telemetrypulse/internal/window"
)

const windowSize = 100

// Config decides how sensitive the detector is. A reading is a spike when it is
// more than ZThreshold stddevs above the baseline AND at least MinJumpMs slower,
// once the site has MinSamples readings.
type Config struct {
	ZThreshold float64
	MinJumpMs  float64
	MinSamples int
}

func DefaultConfig() Config {
	return Config{
		ZThreshold: 2.5,
		MinJumpMs:  10,
		MinSamples: 10,
	}
}

type Result struct {
	ZScore   float64
	Baseline float64 // average latency of the window before this reading
	Stddev   float64 // spread of the window before this reading
	Anomaly  bool
}

type Detector struct {
	cfg     Config
	windows map[string]*window.Window
}

func NewDetector(cfg Config) *Detector {
	return &Detector{
		cfg:     cfg,
		windows: make(map[string]*window.Window),
	}
}

// Analyze should only be called with healthy readings, failed requests
// would drag the baseline down because they come back almost instantly.
func (d *Detector) Analyze(url string, latency float64) Result {
	w := d.windows[url]

	if w == nil {
		// the first request to a site also pays for a cold DNS lookup and TLS
		// setup, on the live server it was 10-75x slower than everything after
		// it (github.com: 300ms, then ~4ms), so it never goes into the baseline
		d.windows[url] = window.MakeWindow(windowSize)
		return Result{}
	}

	mean := w.Mean()
	stddev := w.StandardDev()

	res := Result{
		ZScore:   w.ZScore(latency),
		Baseline: mean,
		Stddev:   stddev,
	}
	res.Anomaly = w.Count() >= d.cfg.MinSamples && res.ZScore > d.cfg.ZThreshold && latency-mean > d.cfg.MinJumpMs

	if res.Anomaly {
		// add a capped value instead of the raw spike, otherwise one 5s timeout
		// blows up the stddev and hides every problem after it
		w.Add(mean + d.cfg.ZThreshold*stddev)
	} else {
		w.Add(latency)
	}

	return res
}
