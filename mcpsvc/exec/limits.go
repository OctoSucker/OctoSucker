package execmcp

type SandboxLimits struct {
	CPUsec       int
	MemoryMB     int
	MaxProcs     int
	MaxOpenFiles int
	Network      string
}
