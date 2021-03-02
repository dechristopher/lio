package str

// Current max caller padding, dynamically increased
var CPadding = 0

// Flags, log formats and miscellaneous strings
const (
	FHealth      = "health"
	FHealthUsage = "Server does not run. Instead, a health" +
		" check runs for any local servers"
	FDebugFlags      = "debug"
	FDebugFlagsUsage = "Comma separated debug flags [foo,bar,baz]"

	InfoFormat  = "INFO  [%s] %s\n"
	DebugFormat = "DEBUG [%s] %s\n"
	ErrorFormat = "ERROR [%s] %s\n"
)

// (C) Log caller names
const (
	CMain    = "LIOInit"
	CLogging = "Logging"
	CPickFS  = "PickFS"
	CWS      = "WS"
)

// (E) Error messages
const (
	ELogFail = "failed to log error=%s msg=%+v"
	EWSRead  = "read err: %s"
	EWSWrite = "read err: %s"
	EWSNoBid = "no bid: %s"
)

// (U) User-facing error messages and codes
const ()

// (M) Standard info log messages
const (
	MDevMode  = "!! DEVELOPER MODE !!"
	MStarted  = "started in %s [env: %s][http: %s][health: %s]"
	MShutdown = "shutting down"
	MExit     = "exit"
)

// (D) Debug log messages
const (
	DPickFSOS = "selected OS - %s"
	DPickFSEm = "selected embedded - %s"
	DWSRecv   = "ws recv: %+v"
	DWSSend   = "ws send: %+v"
)

// (T) Test messages
const ()
