package swiftrt

import (
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

// LoggerLevel is swift-log `Logger.Level`, ordered from most to least verbose.
type LoggerLevel int

// Logger.Level cases.
const (
	LevelTrace LoggerLevel = iota
	LevelDebug
	LevelInfo
	LevelNotice
	LevelWarning
	LevelError
	LevelCritical
)

// Logger.Level cases named through the type, as `Logger.Level.info` is written in full.
const (
	LoggerLevelTrace    = LevelTrace
	LoggerLevelDebug    = LevelDebug
	LoggerLevelInfo     = LevelInfo
	LoggerLevelNotice   = LevelNotice
	LoggerLevelWarning  = LevelWarning
	LoggerLevelError    = LevelError
	LoggerLevelCritical = LevelCritical
)

var loggerLevelNames = [...]string{"trace", "debug", "info", "notice", "warning", "error", "critical"}

// LevelRawValue is swift-log `Logger.Level.rawValue`: the case name, such as "warning".
func LevelRawValue(l LoggerLevel) string { return loggerLevelNames[l] }

// LevelOptInitRawValue is swift-log `Logger.Level(rawValue:)`: nil unless s is a case name.
func LevelOptInitRawValue(s string) *LoggerLevel {
	for i, n := range loggerLevelNames {
		if n == s {
			l := LoggerLevel(i)
			return &l
		}
	}
	return nil
}

// Description is how Swift prints a `Logger.Level`: its case name.
func (l LoggerLevel) Description() string { return LevelRawValue(l) }

// LoggerMessage is swift-log `Logger.Message`, a string built from a literal or interpolation.
type LoggerMessage = string

// LoggerMetadataValue is swift-log `Logger.MetadataValue`.
type LoggerMetadataValue struct {
	kind  byte // 's' string, 'c' stringConvertible, 'd' dictionary, 'a' array
	str   string
	conv  any
	dict  map[string]LoggerMetadataValue
	array []LoggerMetadataValue
}

// LoggerMetadata is swift-log `Logger.Metadata`.
type LoggerMetadata = map[string]LoggerMetadataValue

// MetadataValueString is swift-log `Logger.MetadataValue.string(_:)`.
func MetadataValueString(s string) LoggerMetadataValue { return LoggerMetadataValue{kind: 's', str: s} }

// MetadataValueStringConvertible is swift-log `Logger.MetadataValue.stringConvertible(_:)`.
func MetadataValueStringConvertible(v any) LoggerMetadataValue {
	return LoggerMetadataValue{kind: 'c', conv: v}
}

// MetadataValueDictionary is swift-log `Logger.MetadataValue.dictionary(_:)`.
func MetadataValueDictionary(m LoggerMetadata) LoggerMetadataValue {
	return LoggerMetadataValue{kind: 'd', dict: m}
}

// MetadataValueArray is swift-log `Logger.MetadataValue.array(_:)`.
func MetadataValueArray(a []LoggerMetadataValue) LoggerMetadataValue {
	return LoggerMetadataValue{kind: 'a', array: a}
}

// swiftDebugQuote is Swift `String.debugDescription`: the string in double quotes with `"`, `'`,
// `\`, tab, newline, carriage return and NUL backslash-escaped and other ASCII control characters
// written as `\u{XX}`. Non-ASCII scalars are written as they are.
func swiftDebugQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\'', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case 0:
			b.WriteString(`\0`)
		default:
			if r < 0x20 || r == 0x7f {
				const hex = "0123456789ABCDEF"
				b.WriteString(`\u{`)
				b.WriteByte(hex[r>>4])
				b.WriteByte(hex[r&15])
				b.WriteByte('}')
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Description is swift-log `Logger.MetadataValue.description`: strings as they are, arrays and
// dictionaries in Swift collection syntax with their string leaves quoted. Dictionary entries are
// written in key order, one of the orders Swift's unordered iteration may produce.
func (v LoggerMetadataValue) Description() string {
	switch v.kind {
	case 's':
		return v.str
	case 'c':
		return Str(v.conv)
	case 'a':
		items := make([]string, len(v.array))
		for i, e := range v.array {
			items[i] = e.quotedAsCollectionElement()
		}
		return "[" + strings.Join(items, ", ") + "]"
	}
	if len(v.dict) == 0 {
		return "[:]"
	}
	keys := make([]string, 0, len(v.dict))
	for k := range v.dict {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	entries := make([]string, len(keys))
	for i, k := range keys {
		entries[i] = swiftDebugQuote(k) + ": " + v.dict[k].quotedAsCollectionElement()
	}
	return "[" + strings.Join(entries, ", ") + "]"
}

func (v LoggerMetadataValue) quotedAsCollectionElement() string {
	switch v.kind {
	case 's':
		return swiftDebugQuote(v.str)
	case 'c':
		return swiftDebugQuote(Str(v.conv))
	}
	return v.Description()
}

// LogEvent is swift-log `LogEvent`, the record a LogHandler receives.
type LogEvent struct {
	Level    LoggerLevel
	Message  LoggerMessage
	Error    error
	Metadata *LoggerMetadata
	source   *string
	File     string
	Function string
	Line     uint
}

// LogEventLevel is swift-log `LogEvent.level`.
func LogEventLevel(e LogEvent) LoggerLevel { return e.Level }

// LogEventMessage is swift-log `LogEvent.message`.
func LogEventMessage(e LogEvent) LoggerMessage { return e.Message }

// LogEventMetadata is swift-log `LogEvent.metadata`: the metadata passed with this one log call, or nil.
func LogEventMetadata(e LogEvent) *LoggerMetadata { return e.Metadata }

// LogEventError is swift-log `LogEvent.error`.
func LogEventError(e LogEvent) error { return e.Error }

// LogEventFile is swift-log `LogEvent.file`.
func LogEventFile(e LogEvent) string { return e.File }

// LogEventFunction is swift-log `LogEvent.function`.
func LogEventFunction(e LogEvent) string { return e.Function }

// LogEventLine is swift-log `LogEvent.line`.
func LogEventLine(e LogEvent) uint { return e.Line }

// LogEventSource is swift-log `LogEvent.source`: the explicit source, or else the module named by
// the file ID's first path component, or "n/a" when the file ID has no slash.
func LogEventSource(e LogEvent) string {
	if e.source != nil {
		return *e.source
	}
	if i := strings.IndexByte(e.File, '/'); i >= 0 {
		return e.File[:i]
	}
	return "n/a"
}

// LogHandler is swift-log `LogHandler`. A handler's `logLevel` is read from a `GetLogLevel` method
// when it has one, or else from its `LogLevel` field.
type LogHandler interface {
	Log(event LogEvent)
}

// levelledHandler is a runtime LogHandler that keeps its level itself.
type levelledHandler interface {
	logLevel() LoggerLevel
	withLogLevel(LoggerLevel) LogHandler
}

func handlerLogLevel(h LogHandler) LoggerLevel {
	switch x := h.(type) {
	case levelledHandler:
		return x.logLevel()
	case interface{ GetLogLevel() LoggerLevel }:
		return x.GetLogLevel()
	}
	rv := reflect.ValueOf(h)
	for rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Struct {
		if f := rv.FieldByName("LogLevel"); f.IsValid() && f.Type() == reflect.TypeFor[LoggerLevel]() {
			return LoggerLevel(f.Int())
		}
	}
	return LevelTrace
}

// handlerWithLogLevel is `handler.logLevel = l`: a struct handler is copied, as Swift's value
// semantics copy it, and a class handler is changed in place.
func handlerWithLogLevel(h LogHandler, l LoggerLevel) LogHandler {
	switch x := h.(type) {
	case levelledHandler:
		return x.withLogLevel(l)
	case interface{ SetLogLevel(LoggerLevel) }:
		x.SetLogLevel(l)
		return h
	}
	rv := reflect.ValueOf(h)
	if rv.Kind() == reflect.Pointer {
		if e := rv.Elem(); e.Kind() == reflect.Struct {
			if f := e.FieldByName("LogLevel"); f.CanSet() && f.Type() == reflect.TypeFor[LoggerLevel]() {
				f.SetInt(int64(l))
			}
		}
		return h
	}
	if rv.Kind() == reflect.Struct {
		cp := reflect.New(rv.Type()).Elem()
		cp.Set(rv)
		if f := cp.FieldByName("LogLevel"); f.CanSet() && f.Type() == reflect.TypeFor[LoggerLevel]() {
			f.SetInt(int64(l))
			return cp.Interface().(LogHandler)
		}
	}
	return h
}

// Logger is swift-log `Logger`. Assigning LogLevel is `logger.logLevel = ...`, which applies to the
// logger's own copy of its handler.
type Logger struct {
	Label    string
	LogLevel LoggerLevel
	Handler  LogHandler
	// handlerLevel is the handler's level when the logger last read it, so a LogLevel that differs
	// from it was assigned through the logger.
	handlerLevel LoggerLevel
}

func newLogger(label string, h LogHandler) Logger {
	l := handlerLogLevel(h)
	return Logger{Label: label, LogLevel: l, Handler: h, handlerLevel: l}
}

func (l Logger) log(level LoggerLevel, message string, file string, line uint) {
	h, threshold := l.Handler, handlerLogLevel(l.Handler)
	if l.LogLevel != l.handlerLevel {
		h, threshold = handlerWithLogLevel(h, l.LogLevel), l.LogLevel
	}
	if threshold <= level {
		h.Log(LogEvent{Level: level, Message: message, File: file, Line: line})
	}
}

// LoggerInitLabel is swift-log `Logger(label:)`: a logger whose handler the bootstrapped factory
// makes, a StreamLogHandler on standard error before LoggingSystemBootstrap.
func LoggerInitLabel(label string) Logger {
	loggingSystem.mu.Lock()
	factory := loggingSystem.factory
	loggingSystem.mu.Unlock()
	return newLogger(label, factory(label))
}

// LoggerInitLabelFactory is swift-log `Logger(label:factory:)`.
func LoggerInitLabelFactory(label string, factory func(string) LogHandler) Logger {
	return newLogger(label, factory(label))
}

// The Logger level methods take the caller's file ID and line; the function name the Swift default
// argument supplies is not available, so events carry an empty LogEvent.Function.

// LoggerTraceFileLine is swift-log `Logger.trace(_:file:line:)`.
func LoggerTraceFileLine(l Logger, message string, file string, line uint) {
	l.log(LevelTrace, message, file, line)
}

// LoggerDebugFileLine is swift-log `Logger.debug(_:file:line:)`.
func LoggerDebugFileLine(l Logger, message string, file string, line uint) {
	l.log(LevelDebug, message, file, line)
}

// LoggerInfoFileLine is swift-log `Logger.info(_:file:line:)`.
func LoggerInfoFileLine(l Logger, message string, file string, line uint) {
	l.log(LevelInfo, message, file, line)
}

// LoggerNoticeFileLine is swift-log `Logger.notice(_:file:line:)`.
func LoggerNoticeFileLine(l Logger, message string, file string, line uint) {
	l.log(LevelNotice, message, file, line)
}

// LoggerWarningFileLine is swift-log `Logger.warning(_:file:line:)`.
func LoggerWarningFileLine(l Logger, message string, file string, line uint) {
	l.log(LevelWarning, message, file, line)
}

// LoggerErrorFileLine is swift-log `Logger.error(_:file:line:)`.
func LoggerErrorFileLine(l Logger, message string, file string, line uint) {
	l.log(LevelError, message, file, line)
}

// LoggerCriticalFileLine is swift-log `Logger.critical(_:file:line:)`.
func LoggerCriticalFileLine(l Logger, message string, file string, line uint) {
	l.log(LevelCritical, message, file, line)
}

var loggingSystem = struct {
	mu          sync.Mutex
	factory     func(string) LogHandler
	initialized bool
}{factory: func(label string) LogHandler { return StreamLogHandlerStandardErrorLabel(label) }}

// LoggingSystemBootstrap is swift-log `LoggingSystem.bootstrap(_:)`: Loggers created afterwards get
// their handler from factory. A second call traps, as swift-log's precondition does.
func LoggingSystemBootstrap(factory func(string) LogHandler) {
	loggingSystem.mu.Lock()
	defer loggingSystem.mu.Unlock()
	if loggingSystem.initialized {
		Trap("logging system can only be initialized once per process.")
	}
	loggingSystem.factory, loggingSystem.initialized = factory, true
}

// MultiplexLogHandler is swift-log `MultiplexLogHandler`.
type MultiplexLogHandler struct {
	handlers []LogHandler
	level    LoggerLevel
}

// MultiplexLogHandlerInitSlice is swift-log `MultiplexLogHandler(_:)`: a handler whose level is the
// most verbose of handlers' levels, forwarding each event to every handler whose own level admits it.
func MultiplexLogHandlerInitSlice(handlers []LogHandler) MultiplexLogHandler {
	level := LevelTrace
	for i, h := range handlers {
		if l := handlerLogLevel(h); i == 0 || l < level {
			level = l
		}
	}
	return MultiplexLogHandler{handlers: handlers, level: level}
}

// Log is swift-log `MultiplexLogHandler.log(event:)`.
func (m MultiplexLogHandler) Log(event LogEvent) {
	for _, h := range m.handlers {
		if handlerLogLevel(h) <= event.Level {
			h.Log(event)
		}
	}
}

func (m MultiplexLogHandler) logLevel() LoggerLevel { return m.level }

func (m MultiplexLogHandler) withLogLevel(l LoggerLevel) LogHandler {
	hs := make([]LogHandler, len(m.handlers))
	for i, h := range m.handlers {
		hs[i] = handlerWithLogLevel(h, l)
	}
	return MultiplexLogHandler{handlers: hs, level: l}
}

// SwiftLogNoOpLogHandler is swift-log `SwiftLogNoOpLogHandler`: it discards every event, and its
// level is always `.critical`.
type SwiftLogNoOpLogHandler struct{}

// SwiftLogNoOpLogHandlerInit is swift-log `SwiftLogNoOpLogHandler()`.
func SwiftLogNoOpLogHandlerInit() SwiftLogNoOpLogHandler { return SwiftLogNoOpLogHandler{} }

// Log is swift-log `SwiftLogNoOpLogHandler.log(event:)`, which does nothing.
func (SwiftLogNoOpLogHandler) Log(LogEvent) {}

func (SwiftLogNoOpLogHandler) logLevel() LoggerLevel { return LevelCritical }

func (h SwiftLogNoOpLogHandler) withLogLevel(LoggerLevel) LogHandler { return h }

// StreamLogHandler is swift-log `StreamLogHandler`.
type StreamLogHandler struct {
	label    string
	w        *os.File
	level    LoggerLevel
	Metadata LoggerMetadata
}

var stdioWriteLock sync.Mutex

// StreamLogHandlerStandardErrorLabel is swift-log `StreamLogHandler.standardError(label:)`: a
// handler at level `.info` writing to standard error.
func StreamLogHandlerStandardErrorLabel(label string) StreamLogHandler {
	return StreamLogHandler{label: label, w: os.Stderr, level: LevelInfo}
}

// StreamLogHandlerStandardOutputLabel is swift-log `StreamLogHandler.standardOutput(label:)`: a
// handler at level `.info` writing to standard output.
func StreamLogHandlerStandardOutputLabel(label string) StreamLogHandler {
	return StreamLogHandler{label: label, w: os.Stdout, level: LevelInfo}
}

func prettyLogMetadata(m LoggerMetadata) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + m[k].Description()
	}
	return strings.Join(parts, " ")
}

// Log is swift-log `StreamLogHandler.log(event:)`: one line of local timestamp, level, label, the
// handler's metadata merged with the event's, source and message. An event's error adds
// `error.message` and `error.type` metadata; the type is the Go type name, where Swift writes the
// module-qualified Swift type.
func (h StreamLogHandler) Log(event LogEvent) {
	metadata := h.Metadata
	if event.Metadata != nil && len(*event.Metadata) > 0 || event.Error != nil {
		metadata = LoggerMetadata{}
		for k, v := range h.Metadata {
			metadata[k] = v
		}
		if event.Metadata != nil {
			for k, v := range *event.Metadata {
				metadata[k] = v
			}
		}
		if event.Error != nil {
			metadata["error.message"] = MetadataValueString(Str(event.Error))
			metadata["error.type"] = MetadataValueString(reflect.TypeOf(event.Error).String())
		}
	}
	var b strings.Builder
	b.WriteString(time.Now().Format("2006-01-02T15:04:05-0700"))
	b.WriteByte(' ')
	b.WriteString(LevelRawValue(event.Level))
	if h.label != "" {
		b.WriteByte(' ')
	}
	b.WriteString(h.label)
	b.WriteByte(':')
	if len(metadata) > 0 {
		b.WriteByte(' ')
		b.WriteString(prettyLogMetadata(metadata))
	}
	b.WriteString(" [")
	b.WriteString(LogEventSource(event))
	b.WriteString("] ")
	b.WriteString(event.Message)
	b.WriteByte('\n')
	stdioWriteLock.Lock()
	defer stdioWriteLock.Unlock()
	_, _ = h.w.WriteString(b.String())
}

func (h StreamLogHandler) logLevel() LoggerLevel { return h.level }

func (h StreamLogHandler) withLogLevel(l LoggerLevel) LogHandler {
	h.level = l
	return h
}
