package exitcode

// Exit code types following the contract.

// ExitCode is the exit code type
type ExitCode int

const (
	Success        ExitCode = 0
	GenericError   ExitCode = 1
	Conflicts      ExitCode = 2
	LockActive     ExitCode = 3
	VerifyFailed   ExitCode = 4
	PendingChanges ExitCode = 5
)

// String returns the exit code name
func (e ExitCode) String() string {
	switch e {
	case Success:
		return "Success"
	case GenericError:
		return "GenericError"
	case Conflicts:
		return "Conflicts"
	case LockActive:
		return "LockActive"
	case VerifyFailed:
		return "VerifyFailed"
	case PendingChanges:
		return "PendingChanges"
	default:
		return "Unknown"
	}
}

// Exit codes are part of the public API.
// Use these exclusively via os.Exit(int(code)) from main.
