package gateway_test

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/haveachin/infrared/connection"
	"github.com/haveachin/infrared/gateway"
	"github.com/haveachin/infrared/protocol"
	"github.com/haveachin/infrared/protocol/handshaking"
	"github.com/haveachin/infrared/protocol/login"
	"github.com/haveachin/infrared/server"
)

var (
	testLoginHSID  byte = 5
	testLoginID    byte = 6
	testStatusHSID byte = 10

	testUnboundID byte = 31

	ErrNotImplemented = errors.New("not implemented")
	ErrNoReadLeft     = errors.New("no packets left to read")
)

type testStructWithID interface {
	ID() string
}

type testServer struct {
	id           string
	loginCalled  int
	statusCalled int
}

func (s *testServer) Status(conn connection.StatusConnection) protocol.Packet {
	s.statusCalled++
	return protocol.Packet{}
}

func (s *testServer) Login(conn connection.LoginConnection) error {
	s.loginCalled++
	return nil
}

func (s *testServer) ID() string {
	return s.id
}

// INcomming CONNection, not obvious? Change it!
type testInConn struct {
	writeCount int
	readCount  int

	hs      handshaking.ServerBoundHandshake
	hsPk    protocol.Packet
	reqType connection.RequestType
	loginPK protocol.Packet
}

func (c *testInConn) WritePacket(p protocol.Packet) error {
	c.writeCount++
	return nil
}

func (c *testInConn) ReadPacket() (protocol.Packet, error) {
	c.readCount++
	switch c.readCount {
	case 1:
		return c.hsPk, nil
	case 2:
		return c.loginPK, nil
	default:
		return protocol.Packet{}, nil
	}

}

func (c *testInConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *testInConn) Name() (string, error) {
	return "", ErrNotImplemented
}

func (c *testInConn) HsPk() (protocol.Packet, error) {
	return c.hsPk, nil
}

func (c *testInConn) Hs() (handshaking.ServerBoundHandshake, error) {
	return c.hs, nil // Always returning hs so we can really test the code or it depends on the boolean return
}

func (c *testInConn) LoginStart() (protocol.Packet, error) {
	return protocol.Packet{}, ErrNotImplemented
}

func (c *testInConn) SendStatus(status protocol.Packet) error {
	return ErrNotImplemented
}

func (c *testInConn) Read(b []byte) (n int, err error) {
	return 0, ErrNotImplemented
}

func (c *testInConn) Write(b []byte) (n int, err error) {
	return 0, ErrNotImplemented
}

// Actual test functions
func TestHandleConnection(t *testing.T) {
	loginReq := connection.LoginRequest
	statusReq := connection.StatusRequest
	invalidReq := connection.UnknownRequest

	tt := []struct {
		name          string
		requesteType  connection.RequestType
		numberOfCalls int
		canFindServer bool
	}{
		{
			name:          "cant find server",
			requesteType:  loginReq,
			numberOfCalls: 0,
			canFindServer: false,
		},
		{
			name:          "valid login hs",
			requesteType:  loginReq,
			numberOfCalls: 1,
			canFindServer: true,
		},
		{
			name:          "valid status hs",
			requesteType:  statusReq,
			numberOfCalls: 1,
			canFindServer: true,
		},
		{
			name:          "invalid hs",
			requesteType:  invalidReq,
			numberOfCalls: 0,
			canFindServer: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			conn := &testInConn{reqType: tc.requesteType}
			server := &testServer{}

			serverStore := &gateway.SingleServerStore{}
			if tc.canFindServer {
				serverStore.Server = server
			}

			gateway := gateway.CreateBasicGatewayWithStore(serverStore)
			gateway.HandleConnection(conn)

			amountServerCalls := (server.loginCalled + server.statusCalled) - tc.numberOfCalls
			if amountServerCalls != 0 {
				t.Errorf("Times method called too many (or not enough): %d", amountServerCalls)
			}

		})
	}

}

func TestFindMatchingServer_SingleServerStore(t *testing.T) {
	serverID := "infrared-1"
	serverAddr := "infrared-1"
	expectedServer := &testServer{id: serverID}
	serverStore := &gateway.SingleServerStore{Server: expectedServer}

	data := findServerData{
		store:      serverStore,
		id:         serverID,
		addr:       serverAddr,
		hsDepended: false,
	}

	testFindServer(data, t)
}

func TestFindServer_DefaultServerStore(t *testing.T) {
	serverID := "infrared-1"
	serverAddr := "addr-1"

	serverStore := &gateway.DefaultServerStore{}
	for id := 2; id < 10; id++ {
		serverID := fmt.Sprintf("addr-%d", id)
		server := &testServer{id: serverID}
		serverStore.AddServer(serverID, server)
	}

	server := &testServer{id: serverID}
	serverStore.AddServer(serverAddr, server)

	data := findServerData{
		store:      serverStore,
		id:         serverID,
		addr:       serverAddr,
		hsDepended: true,
	}

	testFindServer(data, t)
}

type findServerData struct {
	store      gateway.ServerStore
	id         string
	addr       string
	hsDepended bool
}

func testFindServer(data findServerData, t *testing.T) {
	unfindableServerAddr := "pls dont use this string as actual server addr"
	expectedServerID := data.id

	type testCase struct {
		withHS     bool
		shouldFind bool
	}
	tt := []testCase{
		{
			withHS:     true,
			shouldFind: true,
		},
	}
	if data.hsDepended {
		tt1 := testCase{withHS: true, shouldFind: false}
		tt2 := testCase{withHS: false, shouldFind: false}
		tt = append(tt, tt1, tt2)
	} else {
		tt1 := testCase{withHS: false, shouldFind: true}
		tt = append(tt, tt1)
	}

	for _, tc := range tt {
		name := fmt.Sprintf("%T - with hs: %t & shouldFind: %t ", data.store, tc.withHS, tc.shouldFind)
		t.Run(name, func(t *testing.T) {
			serverAddr := protocol.String(data.addr)
			if !tc.shouldFind {
				serverAddr = protocol.String(unfindableServerAddr)
			}
			hs := handshaking.ServerBoundHandshake{ServerAddress: serverAddr}
			pk := hs.Marshal()
			hsConn := &testInConn{hsPk: pk, hs: hs}

			receivedServer, ok := data.store.FindServer(hsConn)

			if ok == tc.shouldFind {
				if ok {
					rServer := receivedServer.(testStructWithID)
					if rServer.ID() != expectedServerID {
						t.Logf("expected:\t%v", expectedServerID)
						t.Logf("got:\t\t%v", rServer.ID())
						t.Error("Found a server with a different ID than expected")
					}
				}
			} else if tc.shouldFind {
				t.Error("didnt find server while it should have")
			} else {
				t.Error("did find server while it should NOT have")
			}

		})
	}

}

// This test is meant for testing how it all works together
//  so only the INcomming and OUTgoing connection should be mocked
func TestInToOutBoundry(t *testing.T) {

	wg := sync.WaitGroup{}
	wg.Add(2)
	channel := make(chan struct{})
	go func() {
		wg.Wait()
		t.Log("AFTER WAIT")
		channel <- struct{}{}
	}()

	serverAddr := "infrared.test"
	HsPk := handshaking.ServerBoundHandshake{
		ServerAddress:   protocol.String(serverAddr),
		ServerPort:      25565,
		ProtocolVersion: 754,
		NextState:       2,
	}.Marshal()

	loginPk := login.ServerLoginStart{Name: "infrared"}.Marshal()

	firstPipePk := protocol.Packet{ID: 25, Data: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}}

	inConnWritePackets := []protocol.Packet{HsPk, loginPk, firstPipePk}

	tOutConn := &bTestConnection{wg: &wg}
	tInConn := &bTestConnection{wg: &wg, pks: inConnWritePackets}

	// Setup server stuff
	serverConnFactory := func() connection.ServerConnection {
		return connection.CreateBasicServerConn(tOutConn, protocol.Packet{})
	}

	mcServer := &server.MCServer{
		ConnFactory: serverConnFactory,
	}
	store := &gateway.DefaultServerStore{}
	store.AddServer(serverAddr, mcServer)

	tGateway := gateway.CreateBasicGatewayWithStore(store)

	ipAddr := &net.TCPAddr{IP: []byte{101, 12, 23, 85}, Port: 50674}
	playerConn := connection.CreateBasicPlayerConnection(tInConn, ipAddr)
	outerListener := &testOutLis{conn: playerConn}

	listener := &gateway.BasicListener{Gw: &tGateway, OutListener: outerListener}

	// Start Testing stuff
	listener.Listen()

	timeout := time.After(100 * time.Millisecond)
	select {
	case <-channel:
		t.Log("Tasked finished before timeout")
	case <-timeout:
		t.Log("Tasked timed out")
		t.FailNow() // Dont check other code it didnt finish anyway
	}

	if !(tInConn.readCount == len(inConnWritePackets)) {
		t.Errorf("Read was only called %d times instead of the expected %d", tInConn.readCount, len(inConnWritePackets))
	}

}

// Boundry test struct
type bTestConnection struct {
	//implements interface ServerConnection atm, might change later
	writeCount int
	readCount  int

	pks         []protocol.Packet
	receivedPks []protocol.Packet

	wg         *sync.WaitGroup
	markedDone bool
}

func (c *bTestConnection) WritePacket(p protocol.Packet) error {
	if c.receivedPks == nil {
		c.receivedPks = make([]protocol.Packet, 1)
	}
	c.receivedPks = append(c.receivedPks, p)
	c.writeCount++
	return nil
}

func (c *bTestConnection) ReadPacket() (protocol.Packet, error) {
	if c.readCount == len(c.pks) {
		if !c.markedDone {
			c.wg.Done()
			c.markedDone = true
		}
		return protocol.Packet{}, ErrNoReadLeft
	}
	pkToReturn := c.pks[c.readCount]
	c.readCount++
	return pkToReturn, nil
}

func (c *bTestConnection) Read(b []byte) (n int, err error) {
	if c.readCount == len(c.pks) {
		if !c.markedDone {
			c.wg.Done()
			c.markedDone = true
		}
		return 0, ErrNoReadLeft
	}
	p := c.pks[c.readCount]
	c.readCount++

	pk, _ := p.Marshal()

	for i := 0; i < len(pk); i++ {
		b[i] = pk[i]
	}
	return len(pk), nil
}

func (c *bTestConnection) Write(b []byte) (n int, err error) {
	pk := protocol.Packet{
		ID:   b[1],
		Data: b[2:],
	}
	c.receivedPks = append(c.receivedPks, pk)
	c.writeCount++
	return 0, nil
}