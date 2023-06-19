package actionlogger

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"strconv"
	"sync"

	"golang.org/x/exp/slog"
)

type Handler struct {
	opts    slog.HandlerOptions
	mux     sync.Mutex
	w       io.Writer
	buf     *bytes.Buffer
	handler slog.Handler
}

func NewHandler(w io.Writer, opts *slog.HandlerOptions) *Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
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
	return &Handler{
		opts:    *opts,
		w:       w,
		buf:     &buf,
		handler: handler,
	}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

//nolint:gocritic // implementation of slog.Handler
func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	h.mux.Lock()
	defer h.mux.Unlock()
	var err error
	switch {
	case record.Level < slog.LevelInfo:
		_, err = h.w.Write([]byte("::debug"))
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
	_, err = h.w.Write([]byte("::"))
	if err != nil {
		return err
	}
	err = writeEscaped(h.w, record.Message+" ")
	if err != nil {
		return err
	}
	h.buf.Reset()
	err = h.handler.Handle(ctx, record)
	if err != nil {
		return err
	}
	return writeEscaped(h.w, h.buf.String())
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{
		opts:    h.opts,
		handler: h.handler.WithAttrs(attrs),
	}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{
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
		case ':':
			_, err = w.Write([]byte("%3A"))
		case ',':
			_, err = w.Write([]byte("%2C"))
		default:
			_, err = w.Write([]byte{byte(r)})
		}
	}
	return err
}
