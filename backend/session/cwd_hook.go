package session

// CwdHookInjector is implemented by terminal sessions that can type the WSL
// cwd-reporting hook on demand. WSLSession satisfies it through the embedded
// LocalSession.
//
// The interface lives in a platform-neutral file because app_terminal.go
// type-asserts on it unconditionally; the implementation is Windows-only
// (WSL sessions do not exist elsewhere).
type CwdHookInjector interface {
	InjectCwdHook() error
}
