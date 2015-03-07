package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"regexp"
)

type Room struct {
	name    string
	clients []*Client
}

func (room *Room) AddClient(client *Client) {
	room.clients = append(room.clients, client)
}

func NewRoom(name string) *Room {
	return &Room{
		name:    name,
		clients: nil,
	}
}

type Client struct {
	conn     net.Conn
	incoming chan string
	outgoing chan string
	reader   *bufio.Reader
	writer   *bufio.Writer

	nick string
}

func (client *Client) Read() {
	for {
		s, err := client.reader.ReadString('\n')

		if err != nil {
			client.conn.Close()
			close(client.incoming)
			close(client.outgoing)
			return
		}

		client.incoming <- s
	}
}

func (client *Client) Write() {
	for s := range client.outgoing {
		client.writer.WriteString(s)
		client.writer.Flush()
	}
}

func NewClient(conn net.Conn) *Client {
	c := &Client{
		conn:     conn,
		incoming: make(chan string),
		outgoing: make(chan string),
		reader:   bufio.NewReader(conn),
		writer:   bufio.NewWriter(conn),
	}

	go c.Read()
	go c.Write()

	return c
}

type ChatServer struct {
	clients []*Client
	rooms   map[string]*Room

	incoming chan Command
}

func (server *ChatServer) JoinRoom(name string, client *Client) {
	room, exists := server.rooms[name]

	if !exists {
		room = NewRoom(name)
		server.rooms[name] = room
	}

	room.AddClient(client)
}

func (server *ChatServer) Broadcast(name string, from *Client, msg string) {
	room, exists := server.rooms[name]

	if !exists {
		from.outgoing <- "Error: Room doesn't exist\n"
		return
	}

	if from.nick == "" {
		from.outgoing <- "Error: Must set NICK first\n"
		return
	}

	msgFmt := fmt.Sprintf("%s / %s: %s\n", name, from.nick, msg)

	for _, client := range room.clients {
		client.outgoing <- msgFmt
	}
}

func NewChatServer() *ChatServer {
	return &ChatServer{
		clients:  nil,
		rooms:    make(map[string]*Room),
		incoming: make(chan Command),
	}
}

func (server *ChatServer) HandleConnections(listener net.Listener) {
	go func() {
		for cmd := range server.incoming {
			cmd.Run(server)
		}
	}()

	for {
		conn, err := listener.Accept()

		if err != nil {
			log.Fatal(err)
		}

		client := NewClient(conn)
		server.clients = append(server.clients, client)

		go func() {
			for msg := range client.incoming {
				cmd := parseCommand(client, msg)

				if cmd == nil {
					client.outgoing <- fmt.Sprintf("Error: Invalid cmd: %s", msg)
				} else {
					server.incoming <- cmd
				}
			}
		}()
	}
}

var nickRegexp, _ = regexp.Compile("nick (\\w+)\n$")
var joinRegexp, _ = regexp.Compile("join (\\w+)\n$")
var msgRegexp, _ = regexp.Compile("msg (\\w+) (.+)\n$")

func parseCommand(client *Client, msg string) Command {
	match := nickRegexp.FindStringSubmatch(msg)

	if match != nil {
		return &NickCommand{
			client: client,
			nick:   match[1],
		}
	}

	match = joinRegexp.FindStringSubmatch(msg)

	if match != nil {
		return &JoinCommand{
			client: client,
			room:   match[1],
		}
	}

	match = msgRegexp.FindStringSubmatch(msg)

	if match != nil {
		return &MsgCommand{
			client:  client,
			room:    match[1],
			message: match[2],
		}
	}

	return nil
}

type Command interface {
	Run(server *ChatServer)
}

type NickCommand struct {
	client *Client
	nick   string
}

func (cmd *NickCommand) Run(server *ChatServer) {
	cmd.client.nick = cmd.nick
}

type JoinCommand struct {
	client *Client
	room   string
}

func (cmd *JoinCommand) Run(server *ChatServer) {
	server.JoinRoom(cmd.room, cmd.client)
}

type MsgCommand struct {
	client  *Client
	room    string
	message string
}

func (cmd *MsgCommand) Run(server *ChatServer) {
	server.Broadcast(cmd.room, cmd.client, cmd.message)
}

func main() {
	listener, err := net.Listen("tcp", ":12345")

	if err != nil {
		log.Fatal(err)
	}

	server := NewChatServer()
	server.HandleConnections(listener)
}
