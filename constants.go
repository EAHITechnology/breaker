package breaker

const (
	STATE_CLOSED int32 = iota
	STATE_OPEN
)

const (
	LOCK   int32 = 1
	UNLOCK int32 = 0
)
