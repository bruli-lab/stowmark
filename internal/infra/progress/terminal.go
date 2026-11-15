package progress

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
)

const (
	RefreshInterval = 200 * time.Millisecond
	barWidth        = 24
	clearLine       = "\r\033[K"
)

type Terminal struct {
	output   io.Writer
	now      func() time.Time
	interval time.Duration

	mu         sync.Mutex
	stage      app.ProgressStage
	totalItems int
	totalBytes int64
	startedAt  time.Time
	stop       chan struct{}
	stopped    chan struct{}

	items atomic.Int64
	bytes atomic.Int64
}

func NewTerminal(output io.Writer) *Terminal {
	return newTerminal(output, time.Now, RefreshInterval)
}

func newTerminal(output io.Writer, now func() time.Time, interval time.Duration) *Terminal {
	return &Terminal{output: output, now: now, interval: interval}
}

func (t *Terminal) Start(stage app.ProgressStage, items int, bytes int64) {
	t.Finish()

	t.mu.Lock()
	defer t.mu.Unlock()
	t.stage = stage
	t.totalItems = items
	t.totalBytes = bytes
	t.startedAt = t.now()
	t.items.Store(0)
	t.bytes.Store(0)
	t.stop = make(chan struct{})
	t.stopped = make(chan struct{})
	go t.refresh(t.stop, t.stopped)
}

func (t *Terminal) Advance(items int, bytes int64) {
	t.items.Add(int64(items))
	t.bytes.Add(bytes)
}

func (t *Terminal) Finish() {
	t.mu.Lock()
	stop, stopped := t.stop, t.stopped
	t.stop, t.stopped = nil, nil
	t.mu.Unlock()
	if stop == nil {
		return
	}
	close(stop)
	<-stopped
	_, _ = io.WriteString(t.output, clearLine)
}

func (t *Terminal) refresh(stop <-chan struct{}, stopped chan<- struct{}) {
	defer close(stopped)
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			_, _ = io.WriteString(t.output, clearLine+t.line())
		}
	}
}

func (t *Terminal) line() string {
	t.mu.Lock()
	stage, totalItems, totalBytes, startedAt := t.stage, t.totalItems, t.totalBytes, t.startedAt
	t.mu.Unlock()

	items := t.items.Load()
	bytes := t.bytes.Load()
	elapsed := t.now().Sub(startedAt)

	done, total := float64(items), float64(totalItems)
	if totalBytes > 0 {
		done, total = float64(bytes), float64(totalBytes)
	}
	fraction := 1.0
	if total > 0 {
		fraction = min(done/total, 1)
	}

	parts := []string{
		fmt.Sprintf("%s %3.0f%% %s", stage, fraction*100, bar(fraction)),
		fmt.Sprintf("%d/%d", items, totalItems),
	}
	if totalBytes > 0 {
		parts = append(parts, FormatBytes(bytes)+"/"+FormatBytes(totalBytes))
		if seconds := elapsed.Seconds(); seconds > 0 {
			parts = append(parts, FormatBytes(int64(float64(bytes)/seconds))+"/s")
		}
	}
	if fraction > 0 && fraction < 1 {
		remaining := time.Duration(float64(elapsed) * (1 - fraction) / fraction)
		parts = append(parts, "ETA "+remaining.Round(time.Second).String())
	}
	return strings.Join(parts, "  ")
}

func bar(fraction float64) string {
	filled := int(fraction * barWidth)
	return "[" + strings.Repeat("=", filled) + strings.Repeat(" ", barWidth-filled) + "]"
}

func FormatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	divisor := int64(unit)
	exponent := 0
	for value := size / unit; value >= unit; value /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(divisor), "KMGTPE"[exponent])
}
