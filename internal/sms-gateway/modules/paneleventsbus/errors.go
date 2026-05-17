package paneleventsbus

import "errors"

// ErrBufferFull es el sentinel de back-pressure en Publish — el subscriber
// es lento y su buffer (16 events) está saturado. El evento se droppea
// para ese subscriber específico (los demás siguen recibiendo). Documentar
// el drop con log Zap permite que el operador vea "panel vivo dejó de
// actualizar para conexión X" en lugar de silencio total.
// Fase 3 plan QA 2026-05-17.
var ErrBufferFull = errors.New("paneleventsbus buffer full")
