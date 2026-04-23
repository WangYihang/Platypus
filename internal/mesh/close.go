package mesh

import "net"

func closeConn(conn net.Conn) {
	if conn != nil {
		_ = conn.Close()
	}
}

func closeConns(conns ...net.Conn) {
	for _, conn := range conns {
		closeConn(conn)
	}
}
