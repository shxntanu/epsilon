// Package sandbox defines the epsilond worker sandbox runner contracts.
//
// The package is intentionally stdlib-only and contains no container runtime
// implementation. Production runners can implement Runner while tests and early
// integrations can use NoopRunner.
package sandbox
