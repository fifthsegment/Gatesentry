package gatesentryproxy

type GatesentrySSLHostnameWithDataSize struct {
	Hostname string
	Datasize int64
}

func (g *GatesentrySSLHostnameWithDataSize) String() string {
	return g.Hostname + " " + string(g.Datasize)
}
