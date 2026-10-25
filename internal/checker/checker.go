// Package checker runs local stack probes concurrently.
package checker

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Result is one finished probe.
type Result struct {
	ID       string
	Name     string
	OK       bool
	Detail   string
	Target   string
	Duration time.Duration
}

// Probe is a single diagnostic. Run must return and must respect ctx.
type Probe struct {
	ID     string
	Name   string
	Target string
	Run    func(ctx context.Context) Result
}

// Run starts every probe immediately and closes the returned channel after
// the last result. Completion order follows whichever probe finishes first,
// so a fast port check is not stuck behind a slow daemon call.
func Run(ctx context.Context, probes []Probe) <-chan Result {
	out := make(chan Result, len(probes))
	var wg sync.WaitGroup
	for _, probe := range probes {
		wg.Add(1)
		go func(probe Probe) {
			defer wg.Done()
			out <- safeRun(ctx, probe)
		}(probe)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func safeRun(ctx context.Context, probe Probe) (res Result) {
	defer func() {
		if rec := recover(); rec != nil {
			res = Result{
				ID:     probe.ID,
				Name:   probe.Name,
				Target: probe.Target,
				Detail: fmt.Sprintf("check failed: %v", rec),
			}
		}
	}()
	if probe.Run == nil {
		return Result{
			ID:     probe.ID,
			Name:   probe.Name,
			Target: probe.Target,
			Detail: "check failed",
		}
	}
	res = probe.Run(ctx)
	if res.ID == "" {
		res.ID = probe.ID
	}
	if res.Name == "" {
		res.Name = probe.Name
	}
	if res.Target == "" {
		res.Target = probe.Target
	}
	return res
}
