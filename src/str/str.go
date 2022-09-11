package str

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
	CMain  = "LIO"
	CLog   = "Log"
	CFS    = "FSys"
	CWS    = "WS"
	CWSC   = "WSCm"
	CHMov  = "HMov"
	CStor  = "Stor"
	CBus   = "Bus"
	CProt  = "Prot"
	CEval  = "Eval"
	CEng   = "Engi"
	CGme   = "Game"
	CClk   = "Clck"
	CRoom  = "Room"
	CChan  = "Chan"
	CCrypt = "Cryp"
)

// (E) Error messages
const (
	ELogFail             = "failed to log error=%s msg=%+v"
	EBadSecureKey        = "Invalid or nonexistent secure key error=%s"
	ECipherBlockTooSmall = "cipherText block size is too short"
	EStoreInit           = "failed to init object store error=%s"
	EFSDecode            = "failed to decode path in strictFS path=%s error=%s"
	EWSConn              = "conn err: %s"
	EWSRead              = "read err: %s"
	EWSWrite             = "write err: meta=%+v error=%s"
	EWSNoUid             = "no uid: %s"
	EMoveUnmarshal       = "failed to parse move: move=%+v error=%s"
	ERecord              = "failed to record game error=%s"
	EProtoMarshal        = "failed to marshal protocol message error=%s"
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
	MWSConn   = "[%s @ %s] ws: new conn from %s"
	MWSDisc   = "[%s @ %s] ws: disconnected %s"
)

// (D) Debug log messages
const (
	DPickFSOS   = "selected OS - %s"
	DPickFSEm   = "selected embedded - %s"
	DStoreOk    = "object store online"
	DBusOk      = "event bus online"
	DEngOk      = "engine dispatcher online"
	DWSRecv     = "ws recv: %+v"
	DWSSend     = "ws send: %+v"
	DEngStart   = "eng start: ofen=%s alg=%d"
	DEngSearch  = "eng search: sec=%.3f ofen=%s alg=%d eval=%+v"
	DGameMove   = "game move: move=%s new_ofen=%s"
	DClockEvent = "clock event: type=%d state=%+v"
)

// (T) Test messages
const ()
