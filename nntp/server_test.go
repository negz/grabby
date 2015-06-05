package nntp

import (
	"bytes"
	"crypto/tls"
	"testing"
)

var grabTests = []struct {
	host  string
	port  int
	ms    int
	opts  []ServerOption
	group string
	id    string
}{
	{
		host:  "nntp.fake",
		port:  119,
		ms:    20,
		opts:  []ServerOption{sessionDialer(fakeDialer)},
		group: "alt.bin.dickbutts",
		id:    "dickbutt$!",
	},
	{
		host:  "nntp.fake",
		port:  119,
		ms:    15,
		opts:  []ServerOption{sessionDialer(fakeDialer), Credentials("dick", "butt")},
		group: "alt.bin.dickbutts",
		id:    "dickbutt$!",
	},
	{
		host:  "nntp.fake",
		port:  119,
		ms:    1,
		opts:  []ServerOption{sessionDialer(fakeDialer), Credentials("dick", "butt")},
		group: "alt.bin.dickbutts",
		id:    "dickbutt$!",
	},
	{
		host:  "nntp.fake",
		port:  119,
		ms:    1,
		opts:  []ServerOption{sessionDialer(fakeDialer), TLS(new(tls.Config))},
		group: "alt.bin.dickbutts",
		id:    "dickbutt$!",
	},
}

func TestGrab(t *testing.T) {
	for _, tt := range grabTests {
		s, err := NewServer(tt.host, tt.port, tt.ms, tt.opts...)
		srv := s.(*Server)
		if err != nil {
			t.Errorf("NewServer(%v, %v, %v, %v): %v", tt.host, tt.port, tt.ms, tt.opts, err)
			continue
		}

		// There's no sessions to shutdown yet.
		if err = s.Shutdown(nil); err != nil {
			t.Errorf("s.Shutdown(): %v", err)
		}

		// Setup some sessions
		if err = s.HandleGrabs(); err != nil {
			t.Errorf("s.HandleGrabs(): %v", err)
		}

		// We're still working, so this should have no effect until we shutdown
		if err = s.HandleGrabs(); err != nil {
			t.Errorf("s.HandleGrabs(): %v", err)
		}

		// Shutdown fo reals.
		if err = s.Shutdown(nil); err != nil {
			t.Errorf("s.Shutdown(): %v", err)
		}

		// Replace our sessions with newer, better ones.
		if err = s.HandleGrabs(); err != nil {
			t.Errorf("s.HandleGrabs(): %v", err)
		}

		for _, sn := range srv.sessions {
			connected, authenticated, compressed := sn.Status()
			if s.Username() != "" && !authenticated {
				t.Errorf("%+v sn.Status() authenticated == false", sn)
			}
			if !compressed {
				t.Errorf("%+v sn.Status() compressed == false", sn)
			}
			if !connected {
				t.Errorf("%+v sn.Status() connected == false", sn)
			}
		}

		b := new(bytes.Buffer)
		s.Grab(&GrabRequest{Group: tt.group, ID: tt.id, WriteTo: b})
		rsp := <-s.Grabbed()

		if rsp.Error != nil {
			t.Errorf("rsp.Error: %v", rsp.Error)
		}

		if rsp.Bytes != int64(len(tt.id)+2) {
			t.Errorf("rsp.Bytes == %v, wanted %v", rsp.Bytes, len(tt.id)+2)
		}

		if err = s.Err(); err != nil {
			t.Errorf("s.Err(): %v", err)
		}

		// Shutdown fo really reals
		if err = s.Shutdown(nil); err != nil {
			t.Errorf("s.Shutdown(): %v", err)
		}

		for _, sn := range srv.sessions {
			connected, authenticated, compressed := sn.Status()
			if authenticated {
				t.Errorf("%+v sn.Status() authenticated == true", sn)
			}
			if compressed {
				t.Errorf("%+v sn.Status() compressed == true", sn)
			}
			if connected {
				t.Errorf("%+v sn.Status() connected == true", sn)
			}
		}

	}
}
