package syntax

import (
	"os"
	"strconv"
	"time"
)

// Debug knobs for the tree-sitter crash investigation. All are off by
// default and are read once at startup. They apply to every language.
//
//	GE_TS_NOCANCEL=1      A new edit no longer cancels the parse that is in
//	                      flight. (Detach still does.) The parser keeps its
//	                      ProgressCallback, which then never asks to stop.
//	                      Combine it with the parser-side GE_TS_NOCB, which
//	                      removes the ProgressCallback altogether.
//	GE_TS_DEBOUNCE_MS=80  After a wake-up, wait until no further wake-up has
//	                      arrived for this many milliseconds before taking
//	                      the pending changes. Changes that arrive in a burst
//	                      are applied in one batch.
//
// Note: Parser.Edit reparses once per change, so a batch of N changes still
// costs N parses. The debounce only reduces how often a parse is started
// and, therefore, how often one is cancelled.
var (
	dbgNoCancel = os.Getenv("GE_TS_NOCANCEL") != ""
	dbgDebounce = envMillis("GE_TS_DEBOUNCE_MS")
)

func envMillis(name string) time.Duration {
	n, err := strconv.Atoi(os.Getenv(name))
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Millisecond
}

// waitQuiet blocks until no wake-up has arrived for dbgDebounce. It returns
// false if the worker is quitting.
func (w *Worker) waitQuiet() bool {
	t := time.NewTimer(dbgDebounce)
	defer t.Stop()
	for {
		select {
		case <-w.quit:
			return false
		case <-w.wake:
			t.Reset(dbgDebounce) // still typing: restart the quiet period
		case <-t.C:
			return true
		}
	}
}
