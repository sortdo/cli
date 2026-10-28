package ui

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// Spinner shows that sortdo is waiting: five bars that sort themselves, then shuffle and sort again.
type Spinner struct {
	stop chan struct{}
	done sync.WaitGroup
}

// Bars from short to tall. A frame lists bar heights, 0 to 4.
var barGlyphs = []string{"▂", "▃", "▅", "▆", "█"}

// sortingFrames is every step of a bubble sort of each shuffle, with a pause on the sorted bars.
func sortingFrames() [][]int {
	var frames [][]int
	for _, start := range [][]int{{3, 0, 4, 1, 2}, {4, 2, 0, 3, 1}, {1, 4, 3, 0, 2}} {
		bars := append([]int(nil), start...)
		frames = append(frames, append([]int(nil), bars...))
		for sorted := false; !sorted; {
			sorted = true
			for i := 0; i+1 < len(bars); i++ {
				if bars[i] > bars[i+1] {
					bars[i], bars[i+1] = bars[i+1], bars[i]
					frames = append(frames, append([]int(nil), bars...))
					sorted = false
				}
			}
		}
		for range 4 {
			frames = append(frames, append([]int(nil), bars...))
		}
	}
	return frames
}

func (s Style) frame(bars []int) string {
	var out strings.Builder
	for _, height := range bars {
		out.WriteString(s.paint(fmt.Sprintf("38;5;%d", blues[min(len(blues)-1, (4-height)*len(blues)/5)]), barGlyphs[height]))
	}
	return out.String()
}

// Spin shows the spinner with a label on w until Stop. A plain style shows nothing. The spinner waits a moment before
// it shows, so quick answers do not flicker.
func (s Style) Spin(w io.Writer, label string) *Spinner {
	spinner := &Spinner{stop: make(chan struct{})}
	if !s.on {
		return spinner
	}
	spinner.done.Add(1)
	go func() {
		defer spinner.done.Done()
		select {
		case <-spinner.stop:
			return
		case <-time.After(150 * time.Millisecond):
		}
		frames := sortingFrames()
		ticker := time.NewTicker(90 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; ; i++ {
			fmt.Fprintf(w, "\r%s %s\x1b[K", s.frame(frames[i%len(frames)]), s.Dim(label))
			select {
			case <-spinner.stop:
				fmt.Fprint(w, "\r\x1b[K")
				return
			case <-ticker.C:
			}
		}
	}()
	return spinner
}

// Stop removes the spinner. It is safe to call more than once.
func (spinner *Spinner) Stop() {
	select {
	case <-spinner.stop:
	default:
		close(spinner.stop)
	}
	spinner.done.Wait()
}
