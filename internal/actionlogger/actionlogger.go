package actionlogger

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/exp/slog"
)

type Options struct {
	// AddSource causes the handler to compute the source code position
	// of the log statement and add a SourceKey attribute to the output.
	AddSource bool

	// Level reports the minimum record level that will be logged.
	// The handler discards records with lower levels.
	// If Level is nil, the handler assumes LevelInfo.
	// The handler calls Level.Level for each record processed;
	// to adjust the minimum level dynamically, use a LevelVar.
	Level slog.Leveler
}

type Handler struct {
	opts   Options
	attrs  []slog.Attr
	groups []string
	mux    sync.Mutex
	w      io.Writer
}

var _ slog.Handler = &Handler{}

func NewHandler(w io.Writer, opts *Options) *Handler {
	h := Handler{
		w: w,
	}
	if opts != nil {
		h.opts = *opts
	}
	return &h
}

func (h *Handler) clone() *Handler {
	return &Handler{
		opts:   h.opts,
		attrs:  append([]slog.Attr{}, h.attrs...),
		groups: append([]string{}, h.groups...),
		w:      h.w,
	}
}

//nolint:gocritic // we need this huge param to implement slog.Handler
func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	if !h.Enabled(ctx, record.Level) {
		return nil
	}
	output := ""
	switch {
	case record.Level < slog.LevelInfo:
		output = "::debug "
	case record.Level < slog.LevelWarn:
		output = "::notice "
	case record.Level < slog.LevelError:
		output = "::warn "
	default:
		output = "::error "
	}
	needsComma := false
	var buf bytes.Buffer
	if len(h.attrs) > 0 || record.NumAttrs() > 0 || h.opts.AddSource {
		output += " "
	}
	if h.opts.AddSource {
		frames := runtime.CallersFrames([]uintptr{record.PC})
		frame, _ := frames.Next()
		output += "file=" + escapeString(frame.File, &buf)
		needsComma = true
		if frame.Line > 0 {
			output += ",line=" + strconv.Itoa(frame.Line)
		}
	}
	for _, attr := range h.attrs {
		attr.Value = attr.Value.Resolve()
		if needsComma {
			output += ","
		}
		output += attr.Key + "=" + escapeString(attr.Value.String(), &buf)
		needsComma = true
	}
	record.Attrs(func(attr slog.Attr) bool {
		attr.Value = attr.Value.Resolve()
		if needsComma {
			output += ","
		}
		key := strings.Join(h.groups, ".")
		if key != "" {
			key += "."
		}
		key += attr.Key
		output += key + "=" + escapeString(attr.Value.String(), &buf)
		needsComma = true
		return true
	})

	output += "::" + record.Message + lineEnding
	h.mux.Lock()
	defer h.mux.Unlock()
	_, err := h.w.Write([]byte(output))
	return err
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := h.clone()
	for _, attr := range attrs {
		h2.attrs = append(h2.attrs, slog.Attr{
			Key:   strings.Join(append(h.groups, attr.Key), "."),
			Value: attr.Value,
		})
	}
	return h2
}

func (h *Handler) WithGroup(name string) slog.Handler {
	h2 := h.clone()
	h2.groups = append(h2.groups, name)
	return h2
}

func escapeString(val string, buf *bytes.Buffer) string {
	if buf == nil {
		buf = bytes.NewBuffer(nil)
	}
	buf.Reset()
	for _, r := range val {
		switch r {
		case '\n':
			buf.WriteString("%0A")
		case '\r':
			buf.WriteString("%0D")
		case '%':
			buf.WriteString("%25")
		case ':':
			buf.WriteString("%3A")
		case ',':
			buf.WriteString("%2C")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
