package resp

import (
	"fmt"
	"github.com/mshaverdo/radish/api"
	"github.com/mshaverdo/radish/log"
	"github.com/mshaverdo/radish/message"
	"github.com/tidwall/redcon"
	"strings"
)

type Server struct {
	host           string
	port           int
	server         *redcon.Server
	messageHandler api.MessageHandler
	stopChan       chan struct{}
}

// NewServer Returns new instance of Server
func NewServer(host string, port int, messageHandler api.MessageHandler) *Server {
	s := Server{
		messageHandler: messageHandler,
		stopChan:       make(chan struct{}),
		host:           host,
		port:           port,
	}

	s.server = redcon.NewServerNetwork(
		"tcp",
		fmt.Sprintf("%s:%d", s.host, s.port),
		s.handler,
		nil, //func(conn redcon.Conn) bool { return true },
		nil,
	)

	return &s
}

// ListenAndServe statrs listening to incoming connections
func (s *Server) ListenAndServe() error {
	err := s.server.ListenAndServe()

	if err == nil {
		<-s.stopChan // wait for full shutdown
		return nil
	} else {
		return err
	}
}

// Stops accepting new requests by Resp server, but not causes return from ListenAndServe() until Shutdown()
func (s *Server) Stop() error {
	return s.server.Close()
}

// Shutdown gracefully shuts server down
func (s *Server) Shutdown() error {
	defer close(s.stopChan)
	return s.Stop()
}

func (s *Server) handler(conn redcon.Conn, command redcon.Command) {
	pipelineCommands := conn.ReadPipeline()
	unreliable := len(pipelineCommands) > 0

	s.processRequest(conn, command, unreliable)
	for _, c := range pipelineCommands {
		s.processRequest(conn, c, unreliable)
	}
}

func (s *Server) processRequest(conn redcon.Conn, command redcon.Command, unreliable bool) {
	argsCount := len(command.Args)
	if argsCount == 0 {
		// redcon souldn't pass empty commands here, but...
		return
	}

	cmd := strings.ToUpper(string(command.Args[0]))
	// handle some RESP-level service commands here
	switch cmd {
	case "PING":
		conn.WriteString("PONG")
		return
	case "QUIT":
		conn.WriteString("OK")
		conn.Close()
		return
	}

	//log.Debugf("Received request: %q", command.Args)

	request := message.NewRequest(cmd, command.Args[1:])
	request.Unreliable = unreliable

	//log.Debugf("Handling request: %s", request)

	response := s.messageHandler.HandleMessage(request)

	//log.Debugf("Sending response: %s", response)

	err := sendResponse(response, conn)
	if err != nil {
		log.Errorf("Sending response failed: %s", err)
	}
}

func sendResponse(response message.Response, conn redcon.Conn) error {
	switch concreteResponse := response.(type) {
	case *message.ResponseStatus:
		switch concreteResponse.Status() {
		case message.StatusOk:
			conn.WriteString("OK")
		case message.StatusNotFound:
			conn.WriteNull()
		case message.StatusTypeMismatch:
			conn.WriteError("WRONGTYPE Operation against a key holding the wrong kind of value")
		default:
			conn.WriteError("ERR " + concreteResponse.Payload())
		}
	case *message.ResponseString:
		conn.WriteBulk(concreteResponse.Payload())
	case *message.ResponseStringSlice:
		conn.WriteArray(len(concreteResponse.Payload()))
		for _, v := range concreteResponse.Payload() {
			conn.WriteBulk(v)
		}
	case *message.ResponseInt:
		conn.WriteInt(concreteResponse.Payload())
	default:
		return fmt.Errorf("unknown response type: %T", response)
	}

	return nil
}
