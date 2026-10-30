// Copyright 2026 The frp Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sharedconn

import (
	"bytes"
	"io"
	"net"
)

// Conn is a net.Conn that replays the bytes read through the reader returned by
// NewConn before it reads from the underlying connection. It lets a caller peek
// at the first bytes to classify the connection and still hand the whole stream
// to the next reader.
type Conn struct {
	net.Conn
	buf *bytes.Buffer
}

// NewConn returns a Conn and a reader tee-ing into its buffer. bufSize only
// presizes the buffer; more bytes can be retained.
func NewConn(conn net.Conn, bufSize int) (*Conn, io.Reader) {
	sc := &Conn{
		Conn: conn,
		buf:  bytes.NewBuffer(make([]byte, 0, bufSize)),
	}
	return sc, io.TeeReader(conn, sc.buf)
}

// Read serves the replayed bytes first, then the underlying connection.
// It is not safe for concurrent use.
func (sc *Conn) Read(p []byte) (n int, err error) {
	if sc.buf == nil {
		return sc.Conn.Read(p)
	}
	n, err = sc.buf.Read(p)
	if err == io.EOF {
		sc.buf = nil
		var n2 int
		n2, err = sc.Conn.Read(p[n:])
		n += n2
	}
	return
}
