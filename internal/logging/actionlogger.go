package logging

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"strconv"
	"sync"

	"golang.org/x/exp/slog"
)

type contextKey string

const loggerKey contextKey = "logger"

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func GetLogger(ctx context.Context) *slog.Logger {
	logger, ok := ctx.Value(loggerKey).(*slog.Logger)
	if !ok {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return logger
}

type ActionHandlerOptions struct {
	slog.HandlerOptions
	// write debug to ::notice instead of ::debug
	DebugToNotice bool
}

type ActionHandler struct {
	opts    ActionHandlerOptions
	mux     sync.Mutex
	w       io.Writer
	buf     *bytes.Buffer
	handler slog.Handler
}

func NewActionHandler(w io.Writer, opts *ActionHandlerOptions) *ActionHandler {
	if opts == nil {
		opts = &ActionHandlerOptions{}
	}
	replace := func(groups []string, attr slog.Attr) slog.Attr {
		if opts.ReplaceAttr != nil {
			attr = opts.ReplaceAttr(groups, attr)
		}
		if attr.Key == "time" || attr.Key == "level" || attr.Key == "msg" {
			return slog.Attr{}
		}
		return attr
	}
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		AddSource:   false,
		Level:       opts.Level,
		ReplaceAttr: replace,
	})
	return &ActionHandler{
		opts:    *opts,
		w:       w,
		buf:     &buf,
		handler: handler,
	}
}

func (h *ActionHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *ActionHandler) Handle(ctx context.Context, record slog.Record) error {
	h.mux.Lock()
	defer h.mux.Unlock()
	var err error
	switch {
	case record.Level < slog.LevelInfo:
		prefix := "::debug"
		if h.opts.DebugToNotice {
			prefix = "::notice"
		}
		_, err = h.w.Write([]byte(prefix))
	case record.Level < slog.LevelWarn:
		_, err = h.w.Write([]byte("::notice"))
	case record.Level < slog.LevelError:
		_, err = h.w.Write([]byte("::warn"))
	default:
		_, err = h.w.Write([]byte("::error"))
	}
	if err != nil {
		return err
	}
	if h.opts.AddSource {
		frames := runtime.CallersFrames([]uintptr{record.PC})
		frame, _ := frames.Next()
		_, err = h.w.Write([]byte(" file="))
		if err != nil {
			return err
		}
		err = writeEscaped(h.w, frame.File)
		if err != nil {
			return err
		}
		if frame.Line > 0 {
			_, err = h.w.Write([]byte(",line=" + strconv.Itoa(frame.Line)))
			if err != nil {
				return err
			}
		}
	}
	err = writeEscaped(h.w, "::"+record.Message+" ")
	if err != nil {
		return err
	}
	h.buf.Reset()
	err = h.handler.Handle(ctx, record)
	if err != nil {
		return err
	}
	_, err = h.w.Write(h.buf.Bytes())
	return err
}

func (h *ActionHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ActionHandler{
		opts:    h.opts,
		handler: h.handler.WithAttrs(attrs),
	}
}

func (h *ActionHandler) WithGroup(name string) slog.Handler {
	return &ActionHandler{
		opts:    h.opts,
		handler: h.handler.WithGroup(name),
	}
}

func writeEscaped(w io.Writer, val string) error {
	var err error
	for _, r := range val {
		switch r {
		case '\n':
			_, err = w.Write([]byte("%0A"))
		case '\r':
			_, err = w.Write([]byte("%0D"))
		case '%':
			_, err = w.Write([]byte("%25"))
		default:
			_, err = w.Write([]byte{byte(r)})
		}
	}
	return err
}
