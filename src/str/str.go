package str

// CPadding is the current max caller padding, dynamically increased
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
	CMain = "LIO"
	CLog  = "Log"
	CFS   = "FSys"
	CTool = "Tool"
	CWS   = "WS"
	CWSC  = "WSCm"
	CHMov = "HMov"
	CStor = "Stor"
	CBus  = "Bus"
	CProt = "Prot"
	CEval = "Eval"
	CEng  = "Engi"
	CGme  = "Game"
)

// (E) Error messages
const (
	ELogFail       = "failed to log error=%s msg=%+v"
	EStoreInit     = "failed to init object store error=%s"
	EFSDecode      = "failed to decode path in strictFS path=%s error=%s"
	EWSRead        = "read err: %s"
	EWSWrite       = "write err: meta=%+v error=%s"
	EWSNoBid       = "no bid: %s"
	EMoveUnmarshal = "failed to parse move: move=%+v error=%s"
	ERecord        = "failed to record game error=%s"
	EProtoMarshal  = "failed to marshal protocol message error=%s"
)

// (U) User-facing error messages and codes
const ()

// (M) Standard info log messages
const (
	MDevMode  = "!! DEVELOPER MODE !!"
	MInit     = "starting %s"
	MStarted  = "started in %s [env: %s][http: %s][health: %s]"
	MShutdown = "shutting down"
	MExit     = "exit"
)

// (D) Debug log messages
const (
	DPickFSOS  = "selected OS - %s"
	DPickFSEm  = "selected embedded - %s"
	DNaughty   = "loaded naughty.txt: %d words"
	DStoreOk   = "object store online"
	DBusOk     = "event bus online"
	DWSRecv    = "ws recv: %+v"
	DWSSend    = "ws send: %+v"
	DEngStart  = "eng start: ofen=%s alg=%d"
	DEngSearch = "eng search: sec=%.3f ofen=%s alg=%d eval=%+v"
	DGameMove  = "game move: move=%s new_ofen=%s eval=%.3f"
)

// (T) Test messages
const ()
