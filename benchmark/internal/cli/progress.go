package cli

import (
	"fmt"
	"sync"
	"time"
)

var spinnerChars = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

type ProgressSpinner struct {
	mu           sync.Mutex
	spinnerIndex int
	startTime    time.Time
	message      string
	endpointDone int
	endpointTot  int
	sequenceDone int
	sequenceTot  int
	running      bool
	stopCh       chan struct{}
	doneCh       chan struct{}
}

func NewProgressSpinner() *ProgressSpinner {
	return &ProgressSpinner{
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

func (p *ProgressSpinner) Start(endpointCount, sequenceCount int) {
	p.mu.Lock()
	p.startTime = time.Now()
	p.endpointTot = endpointCount
	p.sequenceTot = sequenceCount
	p.endpointDone = 0
	p.sequenceDone = 0
	p.message = ""
	p.running = true
	p.mu.Unlock()

	go p.run()
}

func (p *ProgressSpinner) run() {
	defer close(p.doneCh)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			p.clearLine()
			return
		case <-ticker.C:
			p.render()
		}
	}
}

func (p *ProgressSpinner) render() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}

	spinner := spinnerChars[p.spinnerIndex]
	p.spinnerIndex = (p.spinnerIndex + 1) % len(spinnerChars)

	elapsed := time.Since(p.startTime)
	mins := int(elapsed.Minutes())
	secs := int(elapsed.Seconds()) % 60

	line := fmt.Sprintf("%s  %c %s  [%d/%d endpoints]  [%d/%d sequences]  elapsed: %dm%02ds",
		Indent,
		spinner,
		p.message,
		p.endpointDone, p.endpointTot,
		p.sequenceDone, p.sequenceTot,
		mins, secs,
	)
	p.mu.Unlock()

	fmt.Printf("\r\033[K%s", line)
}

func (p *ProgressSpinner) clearLine() {
	fmt.Print("\r\033[K")
}

func (p *ProgressSpinner) UpdateEndpoint(method, path string, done int) {
	p.mu.Lock()
	p.message = fmt.Sprintf("Testing %s %s...", method, path)
	p.endpointDone = done
	p.mu.Unlock()
}

func (p *ProgressSpinner) UpdateSequence(seqName string, done int) {
	p.mu.Lock()
	p.message = fmt.Sprintf("Testing sequence %s...", seqName)
	p.sequenceDone = done
	p.mu.Unlock()
}

func (p *ProgressSpinner) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.running = false
	p.mu.Unlock()

	close(p.stopCh)
	<-p.doneCh
}
