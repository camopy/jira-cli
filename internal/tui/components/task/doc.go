// Package task is the generation-tracked async work manager for a Bubble Tea
// app: named scopes group work, every start bumps the scope's monotonic
// generation, and a finished result is accepted only while its generation is
// still the latest — so a superseded fetch (the user refreshed again, changed
// filters, moved on) is dropped instead of overwriting newer state. All
// bookkeeping happens on the single Update goroutine, so there is no locking;
// the run closure executes off-loop in the returned command and never touches
// the manager. There is deliberately no "task started" message: the caller
// sets its own loading state synchronously when it starts work, which avoids
// any ordering race against the finished message for fast tasks.
package task
