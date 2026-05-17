package sse

import "errors"

var (
	ErrNoConnection = errors.New("no connection")
	// ErrBufferFull es retornado por Send cuando el channel del subscriber
	// está saturado y se aplica back-pressure observable. Antes (pre Fase 3
	// plan QA) este caso solo emitía un log warn — el caller no se enteraba.
	ErrBufferFull = errors.New("sse buffer full")
)
