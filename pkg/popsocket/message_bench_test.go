package popsocket

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ipc "github.com/sonastea/kpoppop-grpc/ipc/go"
)

// Heap and goroutine profiles are optional and captured while workers are
// active. Profile runs should be separate from repeated timing measurements.
func monitorSaveBenchmark(b *testing.B) func() {
	b.Helper()
	dir := os.Getenv("POPSOCKET_PROFILE_DIR")
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
	}
	idle := runtime.NumGoroutine()
	peak := idle
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		profile := time.NewTicker(5 * time.Second)
		defer profile.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				peak = max(peak, runtime.NumGoroutine())
			case <-profile.C:
				if dir == "" {
					continue
				}
				runtime.GC()
				for _, name := range []string{"heap", "goroutine"} {
					path := filepath.Join(dir, strings.ReplaceAll(b.Name(), "/", "-")+"."+name+".pprof")
					file, err := os.Create(path)
					if err != nil {
						b.Error(err)
						continue
					}
					if err := pprof.Lookup(name).WriteTo(file, 0); err != nil {
						b.Error(err)
					}
					if err := file.Close(); err != nil {
						b.Error(err)
					}
				}
			}
		}
	}()
	return func() {
		close(stop)
		<-done
		b.ReportMetric(float64(idle), "goroutines-idle")
		b.ReportMetric(float64(peak), "goroutines-peak")
		b.ReportMetric(float64(runtime.NumGoroutine()), "goroutines-end")
	}
}

func benchmarkSave(b *testing.B, workload string, parallel bool, deliver func(*ipc.Message)) {
	b.Helper()
	pool, counts := newSavePostgres(b)
	store := NewMessageStore(nil, pool)
	// Open the whole pool and prepare the Save statements before measuring.
	var warm sync.WaitGroup
	for range 8 {
		warm.Go(func() {
			_, err := store.Save(b.Context(), saveTestMessage("existing"))
			if err != nil {
				b.Error(err)
			}
		})
	}
	warm.Wait()
	if b.Failed() {
		b.FailNow()
	}
	counts.reset()
	stopMonitor := monitorSaveBenchmark(b)
	var sequence, latency atomic.Int64
	operation := func() {
		id := sequence.Add(1) - 1
		convid := "existing"
		switch workload {
		case "new":
			convid = "new-" + strconv.FormatInt(id, 10)
		case "contended_creation":
			convid = "shared-" + strconv.FormatInt(id/8, 10)
		}
		msg := saveTestMessage(convid)
		start := time.Now()
		saved, err := store.Save(b.Context(), msg)
		if err != nil {
			b.Error(err)
			return
		}
		if deliver != nil {
			deliver(saved)
		}
		latency.Add(time.Since(start).Nanoseconds())
	}
	b.ReportAllocs()
	if parallel {
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				operation()
			}
		})
		b.StopTimer()
	} else {
		for b.Loop() {
			operation()
		}
	}
	stopMonitor()
	ops := float64(sequence.Load())
	b.ReportMetric(float64(latency.Load())/ops, "latency-ns/op")
	b.ReportMetric(float64(counts.operations.Load())/ops, "db-ops/op")
	b.ReportMetric(float64(counts.calls.Load())/ops, "db-calls/op")
	b.ReportMetric(float64(counts.begins.Load())/ops, "tx/op")
	b.ReportMetric(float64(counts.rollbacks.Load())/ops, "rollbacks/op")
}

func BenchmarkMessageStoreSavePostgres(b *testing.B) {
	for _, workload := range []string{"new", "existing"} {
		b.Run(workload, func(b *testing.B) {
			b.Run("serial", func(b *testing.B) { benchmarkSave(b, workload, false, nil) })
			b.Run("concurrent", func(b *testing.B) { benchmarkSave(b, workload, true, nil) })
		})
	}
	b.Run("contended_creation", func(b *testing.B) { benchmarkSave(b, "contended_creation", true, nil) })
}

func BenchmarkMessageDeliveryPostgres(b *testing.B) {
	for _, saturated := range []bool{false, true} {
		name, capacity := "healthy", int(MaxMessageSize)
		if saturated {
			name, capacity = "saturated", 1
		}
		b.Run(name, func(b *testing.B) {
			p := &PopSocket{clients: make(map[int32]map[string]client)}
			var readers sync.WaitGroup
			for _, id := range []int32{1, 2} {
				c := &Client{UserID: id, send: make(chan []byte, capacity)}
				p.clients[id] = map[string]client{"connection": c}
				readers.Go(func() {
					if saturated {
						ticker := time.NewTicker(time.Millisecond)
						defer ticker.Stop()
						for range c.send {
							<-ticker.C
						}
					} else {
						for range c.send {
						}
					}
				})
			}
			// Delivery is synchronous to bound in-flight writers.
			defer func() {
				for _, conns := range p.clients {
					for _, c := range conns {
						close(c.Send())
					}
				}
				readers.Wait()
			}()
			benchmarkSave(b, "existing", true, func(msg *ipc.Message) {
				p.processRegularMessage(toWireFormat(msg), msg)
			})
		})
	}
}
