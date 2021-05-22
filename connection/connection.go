package connection

import (
	"bufio"
	"errors"
	"net"

	"github.com/haveachin/infrared"
	"github.com/haveachin/infrared/protocol"
	"github.com/haveachin/infrared/protocol/handshaking"
)

var (
	ErrCantGetHSPacket = errors.New("cant get handshake packet from caller")
	ErrNoNameYet       = errors.New("we dont have the name of this player yet")
)

type RequestType int8

const (
	UnknownRequest RequestType = 0
	StatusRequest  RequestType = 1
	LoginRequest   RequestType = 2
)

type PipeConnection interface {
	Read(b []byte) (n int, err error)
	Write(b []byte) (n int, err error)
}

type Connection interface {
	infrared.PacketWriter
	infrared.PacketReader
	PipeConnection
}

type HSConnection interface {
	Connection
	Hs() (handshaking.ServerBoundHandshake, error)
	HsPk() (protocol.Packet, error)
	RemoteAddr() net.Addr
}

type LoginConnection interface {
	HSConnection
	Name() (string, error)
	LoginStart() (protocol.Packet, error) // Need more work
}

type StatusConnection interface {
	HSConnection
}

// ServerConnection is the struct/part that creates a connection with the real server
type ServerConnection interface {
	PipeConnection
	Status() (protocol.Packet, error)
	SendPK(pk protocol.Packet) error
}

func CreateBasicPlayerConnection(conn Connection, remoteAddr net.Addr) *BasicPlayerConnection {
	return &BasicPlayerConnection{conn: conn, remoteAddr: remoteAddr}
}

// Basic implementation of LoginConnection
type BasicPlayerConnection struct {
	conn Connection

	remoteAddr net.Addr
	hsPk       protocol.Packet
	loginPk    protocol.Packet
	hs         handshaking.ServerBoundHandshake

	hasHS   bool
	hasHSPk bool
}

func (c *BasicPlayerConnection) ReadPacket() (protocol.Packet, error) {
	pk, err := c.conn.ReadPacket()
	if err != nil {
		return protocol.Packet{}, err
	}
	return pk, nil
}

func (c *BasicPlayerConnection) WritePacket(p protocol.Packet) error {
	return nil
}

func (c *BasicPlayerConnection) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *BasicPlayerConnection) Hs() (handshaking.ServerBoundHandshake, error) {
	if c.hasHS {
		return c.hs, nil
	}

	pk, err := c.HsPk()
	if err != nil {
		return c.hs, err
	}
	c.hs, err = handshaking.UnmarshalServerBoundHandshake(pk)
	if err != nil {
		return c.hs, err
	}
	c.hasHS = true
	return c.hs, nil
}

func (c *BasicPlayerConnection) HsPk() (protocol.Packet, error) {
	if c.hasHSPk {
		return c.hsPk, nil
	}
	pk, err := c.ReadPacket()
	if err != nil {
		return pk, ErrCantGetHSPacket
	}
	c.hsPk = pk
	c.hasHSPk = true
	return pk, nil
}

func (c *BasicPlayerConnection) Name() (string, error) {
	return "", ErrNoNameYet
}

func (c *BasicPlayerConnection) LoginStart() (protocol.Packet, error) {
	pk, _ := c.ReadPacket()
	c.loginPk = pk
	return pk, nil
}

func (c *BasicPlayerConnection) Read(b []byte) (n int, err error) {
	return c.conn.Read(b)
}

func (c *BasicPlayerConnection) Write(b []byte) (n int, err error) {
	return c.conn.Write(b)
}

func CreateBasicServerConn(conn Connection, pk protocol.Packet) ServerConnection {
	return &BasicServerConn{conn: conn, statusPK: pk}
}

type BasicServerConn struct {
	conn     Connection
	statusPK protocol.Packet
}

func (c *BasicServerConn) Status() (protocol.Packet, error) {
	c.conn.WritePacket(c.statusPK)
	return c.conn.ReadPacket()
}

func (c *BasicServerConn) SendPK(pk protocol.Packet) error {
	return c.conn.WritePacket(pk)
}

func (c *BasicServerConn) Read(b []byte) (n int, err error) {
	return c.conn.Read(b)
}

func (c *BasicServerConn) Write(b []byte) (n int, err error) {
	return c.conn.Write(b)
}

func CreateBasicConnection(conn net.Conn) *BasicConnection {
	return &BasicConnection{connection: conn, reader: bufio.NewReader(conn)}
}

type BasicConnection struct {
	connection net.Conn
	reader     protocol.DecodeReader
}

func (c *BasicConnection) WritePacket(p protocol.Packet) error {
	pk, _ := p.Marshal() // Need test for err part of this line
	_, err := c.connection.Write(pk)
	return err
}

func (c *BasicConnection) ReadPacket() (protocol.Packet, error) {
	return protocol.ReadPacket(c.reader)
}

func (c *BasicConnection) Read(b []byte) (n int, err error) {
	return c.connection.Read(b)
}

func (c *BasicConnection) Write(b []byte) (n int, err error) {
	return c.connection.Write(b)
}

func ServerAddr(conn HSConnection) string {
	hs, _ := conn.Hs()
	return string(hs.ServerAddress)
}

func ServerPort(conn HSConnection) int16 {
	hs, _ := conn.Hs()
	return int16(hs.ServerPort)
}

func ProtocolVersion(conn HSConnection) int16 {
	hs, _ := conn.Hs()
	return int16(hs.ProtocolVersion)
}

func ParseRequestType(conn HSConnection) RequestType {
	hs, _ := conn.Hs()
	return RequestType(hs.NextState)
}

func Pipe(c1, c2 PipeConnection) {
	go pipe(c1, c2)
	pipe(c2, c1)
}

func pipe(c1, c2 PipeConnection) {
	buffer := make([]byte, 0xffff)

	for {
		n, err := c1.Read(buffer)
		if err != nil {
			return
		}

		data := buffer[:n]

		_, err = c2.Write(data)
		if err != nil {
			return
		}
	}
}