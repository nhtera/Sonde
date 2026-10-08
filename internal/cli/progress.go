// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/runplan"
)

// ansiYellow is a plain (non-bold) yellow, used only for a retry label in
// the progress bar; every other color in this package reuses logger.go's
// bold constants.
const ansiYellow = "\x1b[33m"

// progressMode is which of the reference CLI's three parallel-run display
// modes a run is in: only progressTestWithBar ever draws a bar, but both
// test modes print a per-file Success/Failure line (printTestLine's own
// rc.test gate already matches that distinction, so progressMode only
// needs to answer "should a bar be drawn").
type progressMode int

const (
	progressDefault progressMode = iota
	progressTestNoBar
	progressTestWithBar
)

// newProgressMode resolves the display mode: a bar is drawn in test mode
// when the progress bar is set on (--progress-bar, HURL_PROGRESS_BAR), or
// when nothing sets it and stderr is a terminal; it is never drawn when it
// is set off (HURL_PROGRESS_BAR=false, the config file's
// --no-progress-bar). Outside test mode there is never a bar, matching
// Mode::new(test, progress_bar) in the reference CLI.
func newProgressMode(test bool, set *bool, stderrIsTTY bool) progressMode {
	switch {
	case !test:
		return progressDefault
	case set != nil && *set, set == nil && stderrIsTTY:
		return progressTestWithBar
	default:
		return progressTestNoBar
	}
}

// resolveProgressBar is the progress bar setting, nil when nothing sets
// it: --progress-bar, else HURL_PROGRESS_BAR, else the config file's
// --no-progress-bar.
func resolveProgressBar(inv *runplan.Invocation, env config.Env, fileOff bool) *bool {
	if inv.Changed("progress-bar") && inv.ProgressBar {
		return new(true)
	}
	if v, ok := env.Bool("PROGRESS_BAR"); ok {
		return &v
	}
	if fileOff {
		return new(false)
	}
	return nil
}

// progressMaxWidth is the terminal width used to wrap an over-long
// progress line, or 0 (no wrapping) when it cannot be determined. The
// reference CLI queries the terminal directly; lacking that dependency
// here, COLUMNS (set by most shells, and by scripts that want wrapping in
// a non-TTY pipe) is used instead.
func progressMaxWidth() int {
	if v := os.Getenv("COLUMNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// jobTotal is the number of jobs a run will execute overall: len(files)
// times --repeat, or infinite when --repeat is -1.
type jobTotal struct {
	n        int
	infinite bool
}

func newJobTotal(fileCount, repeat int) jobTotal {
	if repeat < 0 {
		return jobTotal{infinite: true}
	}
	return jobTotal{n: fileCount * repeat}
}

// format renders the progress bar's "Executed files: ..." header line for
// completed jobs so far, matching the reference CLI's own wording exactly
// (including truncating, not rounding, the percentage).
func (t jobTotal) format(completed int) string {
	if t.infinite {
		return fmt.Sprintf("Executed files: %d\n", completed)
	}
	percent := 0
	if t.n > 0 {
		percent = completed * 100 / t.n
	}
	return fmt.Sprintf("Executed files: %d/%d (%d%%)\n", completed, t.n, percent)
}

// runningJob is one seq's progress, tracked while its job is in flight.
type runningJob struct {
	seq     int
	name    string
	current int
	last    int
	retry   int
}

// progressThrottle limits how often the bar redraws, to avoid flicker:
// ported from parallel/progress.rs's Throttle. The first firstThrottle
// window after start is never throttled (letting the display initialize),
// after which a redraw is allowed only once interval has elapsed since the
// last one.
type progressThrottle struct {
	start         time.Time
	haveLast      bool
	lastUpdate    time.Time
	interval      time.Duration
	firstThrottle time.Duration
}

func newProgressThrottle(interval, firstThrottle time.Duration) progressThrottle {
	return progressThrottle{start: time.Now(), interval: interval, firstThrottle: firstThrottle}
}

func (t *progressThrottle) allowed() bool {
	if !t.haveLast {
		return true
	}
	return time.Since(t.lastUpdate) >= t.interval
}

func (t *progressThrottle) update() {
	if time.Since(t.start) < t.firstThrottle {
		return
	}
	t.lastUpdate, t.haveLast = time.Now(), true
}

func (t *progressThrottle) reset() { t.haveLast = false }

// progressUpdateInterval and progressFirstThrottle are UPDATE_INTERVAL and
// FIRST_THROTTLE in the reference implementation.
const (
	progressUpdateInterval = 100 * time.Millisecond
	progressFirstThrottle  = 16 * time.Millisecond
	// maxRunningDisplayed is MAX_RUNNING_DISPLAYED: how many running jobs
	// the bar shows before collapsing the rest into "...N more".
	maxRunningDisplayed = 8
	// progressBarGraphicWidth is the fixed width, in characters, of the
	// "[===>   ]" portion of a single job's line.
	progressBarGraphicWidth = 24
)

// progressBar is a parallel run's live status display: a port of
// parallel/progress.rs's ParProgress, plus the terminal bookkeeping of
// util/term.rs's Stderr (rewinding the cursor to erase what it last drew).
// Every method no-ops when mode isn't progressTestWithBar, so callers
// (run.go) can invoke it unconditionally.
type progressBar struct {
	mode     progressMode
	color    bool
	maxWidth int
	total    jobTotal

	throttle  progressThrottle
	running   map[int]*runningJob
	completed int
	// lastText is exactly what is currently drawn on the terminal (empty:
	// nothing shown), so it can be erased before the next write without
	// tracking a separate line count.
	lastText string
}

func newProgressBar(mode progressMode, color bool, maxWidth int, total jobTotal) *progressBar {
	return &progressBar{
		mode:     mode,
		color:    color,
		maxWidth: maxWidth,
		total:    total,
		throttle: newProgressThrottle(progressUpdateInterval, progressFirstThrottle),
		running:  map[int]*runningJob{},
	}
}

func (p *progressBar) active() bool { return p.mode == progressTestWithBar }

// onEntryStarted records seq's new current/last entry and retry count,
// then redraws the bar if the refresh throttle allows it — matching the
// reference CLI's WorkerMessage::Running handling exactly, including only
// resetting the throttle's clock when a redraw actually happens. last is
// engine.EntryStarted's own Last field: the engine already knows the
// file's clamped entry count, so there is nothing here to compute or
// re-parse the file for.
func (p *progressBar) onEntryStarted(w io.Writer, seq int, name string, current, last, retry int) {
	if !p.active() {
		return
	}
	job, ok := p.running[seq]
	if !ok {
		job = &runningJob{seq: seq, name: name, last: last}
		p.running[seq] = job
	}
	job.current, job.retry = current, retry
	if !p.throttle.allowed() {
		return
	}
	p.throttle.update()
	if text := p.build(); text != "" {
		p.show(w, text)
	}
}

// onFinished unconditionally hides the bar (the reference CLI's Completed
// handler clears no matter the throttle state, so the completion line that
// follows prints on a clean line) and forces the very next EntryStarted
// redraw to bypass the throttle, so a newly started job's first line
// appears immediately instead of waiting out the interval.
func (p *progressBar) onFinished(w io.Writer, seq int) {
	if !p.active() {
		return
	}
	p.hide(w)
	delete(p.running, seq)
	p.completed++
	p.throttle.reset()
}

// eraseAndReprint runs write with the bar temporarily erased, then redraws
// the same text unchanged: any other stderr output (a log line, a
// warning) must not corrupt the bar, matching Stderr::eprintln/eprint.
func (p *progressBar) eraseAndReprint(w io.Writer, write func()) {
	if !p.active() || p.lastText == "" {
		write()
		return
	}
	p.erase(w)
	write()
	_, _ = io.WriteString(w, p.lastText)
}

func (p *progressBar) show(w io.Writer, text string) {
	p.erase(w)
	_, _ = io.WriteString(w, text)
	p.lastText = text
}

func (p *progressBar) hide(w io.Writer) {
	p.erase(w)
	p.lastText = ""
}

// erase rewinds the cursor over p.lastText's lines and clears them,
// matching Stderr::rewind_cursor exactly (one "cursor up + erase line" per
// newline in the text last drawn).
func (p *progressBar) erase(w io.Writer) {
	if p.lastText == "" {
		return
	}
	if n := strings.Count(p.lastText, "\n"); n > 0 {
		_, _ = io.WriteString(w, strings.Repeat("\x1b[1A\x1b[K", n))
	} else {
		_, _ = io.WriteString(w, "\x1b[K")
	}
}

// build renders the full progress text (the "Executed files: ..." header
// plus one line per displayed running job), or "" when nothing is
// running, matching build_progress in the reference implementation.
func (p *progressBar) build() string {
	running := make([]*runningJob, 0, len(p.running))
	for _, j := range p.running {
		running = append(running, j)
	}
	if len(running) == 0 {
		return ""
	}
	sort.Slice(running, func(i, k int) bool { return running[i].seq < running[k].seq })
	total := len(running)
	displayed := running
	if len(displayed) > maxRunningDisplayed {
		displayed = displayed[:maxRunningDisplayed]
	}

	maxLast := 1
	for _, j := range displayed {
		if j.last > maxLast {
			maxLast = j.last
		}
	}
	maxCompletedWidth := 2*len(strconv.Itoa(maxLast)) + 1

	var b strings.Builder
	b.WriteString(p.total.format(p.completed))
	for _, j := range displayed {
		b.WriteString(p.jobLine(j, maxCompletedWidth))
	}
	if total > maxRunningDisplayed {
		fmt.Fprintf(&b, "...%d more\n", total-maxRunningDisplayed)
	}
	return b.String()
}

// jobLine renders one running job's line: the bar, the padded
// "current/last" counter, "Running <file>" and, if this attempt is a
// retry, "(retry N)".
func (p *progressBar) jobLine(j *runningJob, maxCompletedWidth int) string {
	requests := fmt.Sprintf("%d/%d", j.current, j.last)
	pad := maxCompletedWidth - len(requests)
	if pad < 0 {
		pad = 0
	}

	var line strings.Builder
	line.WriteString(progressBarGraphic(j.current, j.last))
	line.WriteString(strings.Repeat(" ", pad))
	line.WriteByte(' ')
	line.WriteString(p.styled(ansiCyanBold, "Running"))
	line.WriteByte(' ')
	line.WriteString(p.styled(ansiBold, j.name))
	if j.retry > 0 {
		line.WriteByte(' ')
		line.WriteString(p.styled(ansiYellow, fmt.Sprintf("(retry %d)", j.retry)))
	}
	text := line.String()
	// Wrapping needs a plain-text column count; skip it when the line
	// carries ANSI codes rather than mismeasuring it.
	if p.maxWidth > 0 && !p.color {
		text = wrapAtWidth(text, p.maxWidth)
	}
	return text + "\n"
}

func (p *progressBar) styled(style, text string) string {
	if !p.color {
		return text
	}
	return style + text + ansiReset
}

// progressBarGraphic renders the "[===>   ] current/last" bar for one job,
// a direct port of the reference implementation's progress_bar function.
func progressBarGraphic(current, last int) string {
	progress := float64(current-1) / float64(last)
	col := int(progress * float64(progressBarGraphicWidth))
	if col >= progressBarGraphicWidth {
		col = progressBarGraphicWidth - 1
	}
	var completed string
	if col > 0 {
		completed = strings.Repeat("=", col)
	}
	void := strings.Repeat(" ", progressBarGraphicWidth-col-1)
	return fmt.Sprintf("[%s>%s] %d/%d", completed, void, current, last)
}

// wrapAtWidth word-wraps s to width columns, breaking at the last space at
// or before the limit (falling back to a hard break when a single word
// exceeds it).
func wrapAtWidth(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	var b strings.Builder
	for len(s) > width {
		cut := strings.LastIndex(s[:width], " ")
		if cut <= 0 {
			cut = width
		}
		b.WriteString(s[:cut])
		b.WriteByte('\n')
		s = strings.TrimPrefix(s[cut:], " ")
	}
	b.WriteString(s)
	return b.String()
}
