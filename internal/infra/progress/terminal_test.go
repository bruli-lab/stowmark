package progress

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/stretchr/testify/require"
)

type syncBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (s *syncBuffer) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buffer.Write(data)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buffer.String()
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Add(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(duration)
}

func TestTerminal_Line(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		stage    app.ProgressStage
		items    int
		bytes    int64
		advance  [2]int64
		elapsed  time.Duration
		expected string
	}{
		{
			name:     "with bytes half done after 10 seconds, when its line is drawn, then it shows the percentage by bytes, the counts, the speed and the remaining time",
			stage:    app.ProgressBackingUp,
			items:    4,
			bytes:    4 << 20,
			advance:  [2]int64{1, 2 << 20},
			elapsed:  10 * time.Second,
			expected: "Backing up  50% [============            ]  1/4  2.0 MiB/4.0 MiB  204.8 KiB/s  ETA 10s",
		},
		{
			name:     "without bytes, when its line is drawn, then it shows the percentage by items and no sizes",
			stage:    app.ProgressRekeying,
			items:    4,
			advance:  [2]int64{1, 0},
			elapsed:  3 * time.Second,
			expected: "Rekeying  25% [======                  ]  1/4  ETA 9s",
		},
		{
			name:     "with nothing to do, when its line is drawn, then it shows it as complete",
			stage:    app.ProgressVerifying,
			expected: "Verifying 100% [========================]  0/0",
		},
	}
	for _, tt := range tests {
		t.Run(`Given a terminal progress `+tt.name, func(t *testing.T) {
			t.Parallel()

			clock := &fakeClock{now: time.Unix(0, 0)}
			terminal := newTerminal(&syncBuffer{}, clock.Now, time.Hour)
			terminal.Start(tt.stage, tt.items, tt.bytes)
			defer terminal.Finish()

			terminal.Advance(int(tt.advance[0]), tt.advance[1])
			clock.Add(tt.elapsed)

			require.Equal(t, tt.expected, terminal.line())
		})
	}
}

func TestTerminal_Refresh(t *testing.T) {
	t.Parallel()

	t.Run(`Given a started terminal progress,
	when it refreshes and then finishes,
	then it rewrites the same line and clears it at the end`, func(t *testing.T) {
		t.Parallel()

		output := &syncBuffer{}
		terminal := newTerminal(output, time.Now, time.Millisecond)

		terminal.Start(app.ProgressRestoring, 2, 0)
		terminal.Advance(1, 0)
		require.Eventually(t, func() bool { return strings.Contains(output.String(), "Restoring") }, time.Second, time.Millisecond)
		terminal.Finish()
		terminal.Finish()

		written := output.String()
		require.True(t, strings.HasPrefix(written, clearLine))
		require.True(t, strings.HasSuffix(written, clearLine))
		require.NotContains(t, written, "\n")
	})

	t.Run(`Given a terminal progress that is started twice,
	when the second stage starts,
	then the counters start again from zero`, func(t *testing.T) {
		t.Parallel()

		terminal := newTerminal(&syncBuffer{}, time.Now, time.Hour)
		terminal.Start(app.ProgressBackingUp, 1, 0)
		terminal.Advance(1, 0)

		terminal.Start(app.ProgressVerifying, 3, 0)
		defer terminal.Finish()

		require.Contains(t, terminal.line(), "0/3")
	})
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	for size, expected := range map[int64]string{0: "0 B", 1023: "1023 B", 1536: "1.5 KiB", 5 << 30: "5.0 GiB"} {
		require.Equal(t, expected, FormatBytes(size))
	}
}
