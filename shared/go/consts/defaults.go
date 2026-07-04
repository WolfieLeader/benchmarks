package consts

const (
	MaxRequestBytes = 10 << 20 // 10MB global request-body cap
	MaxFileBytes    = 1 << 20  // 1MB
	SniffLen        = 512
	NullByte        = 0x00
	DefaultLimit    = 10
)
