package sse

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/utils"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	eventsBufferSize = 8
)

type Service struct {
	config Config

	mu          sync.RWMutex
	connections map[string][]*sseConnection

	logger *zap.Logger
}

type sseConnection struct {
	id          string
	channel     chan eventWrapper
	closeSignal chan struct{}
}

type eventWrapper struct {
	name string
	data []byte
}

func NewService(config Config, logger *zap.Logger) *Service {
	return &Service{
		config: config,

		mu:          sync.RWMutex{},
		connections: make(map[string][]*sseConnection),

		logger: logger,
	}
}

func (s *Service) Send(deviceID string, event Event) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	connections, exists := s.connections[deviceID]
	if !exists {
		return fmt.Errorf("%w: device %s", ErrNoConnection, deviceID)
	}

	data, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	sent := 0
	bufferFull := false
	for _, conn := range connections {
		select {
		case conn.channel <- eventWrapper{string(event.Type), data}:
			sent++
		case <-conn.closeSignal:
			s.logger.Warn(
				"Connection closed while sending event",
				zap.String("device_id", deviceID),
				zap.String("connection_id", conn.id),
			)
		default:
			bufferFull = true
			s.logger.Warn(
				"Connection buffer full while sending event",
				zap.String("device_id", deviceID),
				zap.String("connection_id", conn.id),
				zap.Error(ErrBufferFull),
			)
		}
	}

	if sent == 0 {
		// Diferenciar back-pressure de "no hay conexión": ambos suben sent=0
		// pero requieren respuestas distintas del caller (retry vs dead-letter).
		// Fase 3 plan QA 2026-05-17.
		if bufferFull {
			return fmt.Errorf("%w: device %s", ErrBufferFull, deviceID)
		}
		return fmt.Errorf("%w: device %s", ErrNoConnection, deviceID)
	}

	return nil
}

func (s *Service) Close(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for deviceID, connections := range s.connections {
		for _, conn := range connections {
			close(conn.closeSignal)
		}
		delete(s.connections, deviceID)
	}
	return nil
}

func (s *Service) Handler(deviceID string, c *fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	c.Status(fiber.StatusOK).Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		s.handleStream(deviceID, w)
	})

	return nil
}

func (s *Service) handleStream(deviceID string, w *bufio.Writer) {
	conn := s.registerConnection(deviceID)
	defer s.removeConnection(deviceID, conn.id)

	var tickerChan <-chan time.Time

	if s.config.keepAlivePeriod > 0 {
		ticker := time.NewTicker(s.config.keepAlivePeriod)
		defer ticker.Stop()

		tickerChan = ticker.C
	}

	for {
		select {
		case event := <-conn.channel:
			if err := s.writeToStream(
				w,
				fmt.Sprintf("event: %s\ndata: %s", event.name, utils.UnsafeString(event.data)),
			); err != nil {
				s.logger.Warn("failed to write event data",
					zap.String("device_id", deviceID),
					zap.String("connection_id", conn.id),
					zap.Error(err))
				return
			}
		case <-tickerChan:
			if err := s.writeToStream(w, ":keepalive"); err != nil {
				s.logger.Warn("failed to write keepalive",
					zap.String("device_id", deviceID),
					zap.String("connection_id", conn.id),
					zap.Error(err))
				return
			}
		case <-conn.closeSignal:
			return
		}
	}
}

func (s *Service) writeToStream(w *bufio.Writer, data string) error {
	if _, err := fmt.Fprintf(w, "%s\n\n", data); err != nil {
		return fmt.Errorf("failed to write to stream: %w", err)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("failed to flush stream: %w", err)
	}

	return nil
}

func (s *Service) registerConnection(deviceID string) *sseConnection {
	s.mu.Lock()
	defer s.mu.Unlock()

	connID := uuid.NewString()

	conn := &sseConnection{
		id:          connID,
		channel:     make(chan eventWrapper, eventsBufferSize),
		closeSignal: make(chan struct{}),
	}

	if _, ok := s.connections[deviceID]; !ok {
		s.connections[deviceID] = []*sseConnection{}
	}

	s.connections[deviceID] = append(s.connections[deviceID], conn)

	s.logger.Info("Registering SSE connection", zap.String("device_id", deviceID), zap.String("connection_id", connID))

	return conn
}

func (s *Service) removeConnection(deviceID, connID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if connections, exists := s.connections[deviceID]; exists {
		for i, conn := range connections {
			if conn.id == connID {
				close(conn.closeSignal)
				s.connections[deviceID] = append(connections[:i], connections[i+1:]...)

				s.logger.Info(
					"Removing SSE connection",
					zap.String("device_id", deviceID),
					zap.String("connection_id", connID),
				)
				break
			}
		}

		if len(s.connections[deviceID]) == 0 {
			delete(s.connections, deviceID)
		}
	}
}
