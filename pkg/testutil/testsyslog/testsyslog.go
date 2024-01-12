/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package testsyslog

import (
	"bufio"
	"crypto/tls"
	"io"
	"log"
	"net"
	"os"
	"runtime"
	"time"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testca"
)

func StartServer(n, la string, done chan<- string, certs ...*testca.Cert) (addr string, sock io.Closer) {
	if n == "udp" || n == "tcp" || n == "tcp+tls" {
		la = "127.0.0.1:0"
	} else {
		// unix and unixgram: choose an address if none given
		if la == "" {
			// use os.CreateTemp to get a name that is unique
			f, err := os.CreateTemp("", "syslogtest")
			if err != nil {
				log.Fatal("TempFile: ", err)
			}
			f.Close()
			la = f.Name()
		}
		os.Remove(la)
	}

	if n == "udp" || n == "unixgram" {
		l, e := net.ListenPacket(n, la)
		if e != nil {
			log.Fatalf("startServer failed: %v", e)
		}
		addr = l.LocalAddr().String()
		sock = l
		go runPacketSyslog(l, done)
	} else if n == "tcp+tls" {
		if len(certs) == 0 {
			log.Fatalf("certificates required.")
		}
		cer := certs[0]
		if cer == nil {
			log.Fatalf("certificates is nil")
		}
		cert, err := tls.LoadX509KeyPair(cer.CertPath, cer.KeyPath)
		if err != nil {
			log.Fatalf("failed to load TLS keypair: %v", err)
		}
		config := tls.Config{Certificates: []tls.Certificate{cert}}
		l, e := tls.Listen("tcp", la, &config)
		if e != nil {
			log.Fatalf("startServer failed: %v", e)
		}
		addr = l.Addr().String()
		sock = l
		go runStreamSyslog(l, done)
	} else {
		l, e := net.Listen(n, la)
		if e != nil {
			log.Fatalf("startServer failed: %v", e)
		}
		addr = l.Addr().String()
		sock = l
		go runStreamSyslog(l, done)
	}
	return addr, sock
}

func TestableNetwork(network string) bool {
	switch network {
	case "unix", "unixgram":
		switch runtime.GOOS {
		case "darwin":
			switch runtime.GOARCH {
			case "arm", "arm64":
				return false
			}
		case "windows":
			return false
		}
	case "udp", "tcp", "tcp+tls":
		return !rootlessutil.IsRootless()
	}
	return true
}

func runPacketSyslog(c net.PacketConn, done chan<- string) {
	var buf [4096]byte
	var rcvd string
	ct := 0
	for {
		var n int
		var err error

		_ = c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, _, err = c.ReadFrom(buf[:])
		rcvd += string(buf[:n])
		if err != nil {
			if oe, ok := err.(*net.OpError); ok {
				if ct < 3 && oe.Temporary() {
					ct++
					continue
				}
			}
			break
		}
	}
	c.Close()
	done <- rcvd
}

func runStreamSyslog(l net.Listener, done chan<- string) {
	for {
		var c net.Conn
		var err error
		if c, err = l.Accept(); err != nil {
			return
		}
		go func(c net.Conn) {
			_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
			b := bufio.NewReader(c)
			for ct := 1; ct&7 != 0; ct++ {
				s, err := b.ReadString('\n')
				if err != nil {
					break
				}
				done <- s
			}
			c.Close()
		}(c)
	}
}
