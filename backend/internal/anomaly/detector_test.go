package anomaly

import (
	"math/rand"
	"testing"
)

func warmUp(d *Detector, url string, n int, r *rand.Rand) {
	for i := 0; i < n; i++ {
		d.Analyze(url, 40+r.NormFloat64()*3)
	}
}

func TestNoAlertsDuringWarmUp(t *testing.T) {
	d := NewDetector(DefaultConfig())
	for i, v := range []float64{40, 40.2, 95, 40.1, 120} {
		if d.Analyze("u", v).Anomaly {
			t.Fatalf("reading %d (%vms) flagged before a baseline exists", i, v)
		}
	}
}

func TestSpikeIsFlagged(t *testing.T) {
	d := NewDetector(DefaultConfig())
	warmUp(d, "u", 100, rand.New(rand.NewSource(1)))

	res := d.Analyze("u", 300)
	if !res.Anomaly {
		t.Fatalf("300ms on a ~40ms baseline not flagged, z=%v", res.ZScore)
	}
	if res.Baseline < 35 || res.Baseline > 45 {
		t.Fatalf("baseline = %v, want ~40", res.Baseline)
	}
}

func TestSmallJumpIsIgnored(t *testing.T) {
	d := NewDetector(DefaultConfig())
	r := rand.New(rand.NewSource(2))
	for i := 0; i < 100; i++ {
		d.Analyze("u", 40+r.NormFloat64()*0.5) // very steady site
	}
	// 4ms above normal is a big z-score here but not a real problem
	if res := d.Analyze("u", 44); res.Anomaly {
		t.Fatalf("4ms jump flagged, z=%v", res.ZScore)
	}
}

func TestOneTimeoutDoesNotHideTheNextSpike(t *testing.T) {
	d := NewDetector(DefaultConfig())
	warmUp(d, "u", 100, rand.New(rand.NewSource(3)))

	if !d.Analyze("u", 5000).Anomaly {
		t.Fatal("5s timeout not flagged")
	}
	if !d.Analyze("u", 300).Anomaly {
		t.Fatal("300ms spike right after a timeout was hidden")
	}
}

func TestSustainedSlowdownKeepsAlertingForAWhile(t *testing.T) {
	d := NewDetector(DefaultConfig())
	r := rand.New(rand.NewSource(4))
	warmUp(d, "u", 100, r)

	flagged := 0
	for i := 0; i < 30; i++ {
		if d.Analyze("u", 160+r.NormFloat64()*3).Anomaly {
			flagged++
		}
	}
	if flagged < 25 {
		t.Fatalf("only %d of 30 readings flagged during a 4x slowdown", flagged)
	}
}

func TestTargetsHaveSeparateBaselines(t *testing.T) {
	d := NewDetector(DefaultConfig())
	r := rand.New(rand.NewSource(5))
	warmUp(d, "fast", 100, r)
	for i := 0; i < 100; i++ {
		d.Analyze("slow", 400+r.NormFloat64()*10)
	}
	if d.Analyze("slow", 410).Anomaly {
		t.Fatal("normal reading for the slow site flagged using the fast site's baseline")
	}
}

func TestStricterConfigFlagsLess(t *testing.T) {
	strict := NewDetector(Config{ZThreshold: 3, MinJumpMs: 20, MinSamples: 20})
	loose := NewDetector(DefaultConfig())
	r := rand.New(rand.NewSource(6))
	for i := 0; i < 100; i++ {
		v := 40 + r.NormFloat64()*3
		strict.Analyze("u", v)
		loose.Analyze("u", v)
	}
	// ~16ms over normal: a spike for the default settings, not for the strict ones
	if !loose.Analyze("u", 56).Anomaly {
		t.Fatal("default config missed a 16ms jump")
	}
	if strict.Analyze("u", 56).Anomaly {
		t.Fatal("strict config flagged a 16ms jump")
	}
}

func TestFirstReadingIsLeftOut(t *testing.T) {
	d := NewDetector(DefaultConfig())
	if res := d.Analyze("u", 300); res.Anomaly || res.Baseline != 0 {
		t.Fatalf("first reading = %+v, want it ignored", res)
	}

	r := rand.New(rand.NewSource(7))
	for i := 0; i < 30; i++ {
		d.Analyze("u", 4+r.NormFloat64()*0.3)
	}

	// baseline is ~4ms, not dragged up by the cold 300ms request
	res := d.Analyze("u", 20)
	if res.Baseline > 5 {
		t.Fatalf("baseline = %v, the cold first reading leaked in", res.Baseline)
	}
	if !res.Anomaly {
		t.Fatalf("20ms on a 4ms site not flagged, z=%v", res.ZScore)
	}
}
