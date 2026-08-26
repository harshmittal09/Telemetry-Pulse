package probe

import (
	"net/http"
	"time"
)

const (
	interval = 500 * time.Millisecond
	timeout  = 5 * time.Second
)

type ProbeResult struct {
	Url        string
	Latency    float64
	StatusCode int
	Timestrap  int64
}

func StartWorker(url string, outChan chan<- ProbeResult) {
	client := &http.Client{
		Timeout: timeout,
	}

	ticker := time.NewTicker(interval)

	go func() {
		for range ticker.C {
			start := time.Now()
			res, err := client.Get(url)

			proberesult := ProbeResult{
				Url:       url,
				Latency:   time.Since(start).Seconds() * 1000,
				Timestrap: time.Now().UnixMilli(),
			}

			if err == nil {
				proberesult.StatusCode = res.StatusCode
				res.Body.Close()
			}

			outChan <- proberesult
		}
	}()
}
