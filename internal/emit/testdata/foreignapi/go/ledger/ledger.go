// Package ledger is the hand-written Go stand-in for the Swift module Ledger in testdata/foreignapi. Its
// names follow the contract swift2go emits for mapped modules.
package ledger

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/ncmud/swift2go/swiftrt"
)

// Book is Ledger's class `Book`, used through a pointer.
type Book struct{ entries []string }

// BookInit is `Book()`.
func BookInit() *Book { return &Book{} }

// BookEntries is `Book.entries`.
func BookEntries(b *Book) []string { return b.entries }

// BookSetEntries sets `Book.entries`.
func BookSetEntries(b *Book, v []string) { b.entries = v }

// BookCount is `Book.count`.
func BookCount(b *Book) int { return len(b.entries) }

// BookWrite is `Book.write(_:)`; T is explicit because it appears only in the result.
func BookWrite[T any](b *Book, body func(*Book) T) T { return body(b) }

// Expressible is the protocol `Expressible`: Swift values of conforming types, held as any.
type Expressible interface{}

// Specific is the protocol `Specific`.
type Specific interface{}

// Expr is `Expr`.
type Expr struct{ sql string }

// ExprSql is `Expr.sql`.
func ExprSql(e Expr) string { return e.sql }

// Column is `Column`.
type Column struct{ name string }

// ColumnInitString is `Column(_:)`.
func ColumnInitString(name string) Column { return Column{name} }

func sqlOf(v any) string {
	switch v := v.(type) {
	case Column:
		return v.name
	case Expr:
		return v.sql
	case string:
		return "'" + v + "'"
	case int:
		return fmt.Sprint(v)
	}
	panic(fmt.Sprintf("ledger: %T is not Expressible", v))
}

// SpecificOpEqual is `Specific.==(_:_: Expressible?)`.
func SpecificOpEqual(l Specific, r Expressible) Expr {
	if r == nil {
		return Expr{sqlOf(l) + " IS NULL"}
	}
	return Expr{sqlOf(l) + " = " + sqlOf(r)}
}

// SpecificOpEqualBool is `Specific.==(_:_: Bool)`.
func SpecificOpEqualBool(l Specific, r bool) Expr {
	if r {
		return Expr{sqlOf(l) + " = 1"}
	}
	return Expr{sqlOf(l) + " = 0"}
}

// SpecificOpAnd is `Specific.&&`.
func SpecificOpAnd(l Specific, r Expr) Expr { return Expr{"(" + sqlOf(l) + " AND " + r.sql + ")"} }

// SpecificDesc is `Specific.desc`.
func SpecificDesc(s Specific) Expr { return Expr{sqlOf(s) + " DESC"} }

// OpOr is the module-level `||`.
func OpOr(l, r Expr) Expr { return Expr{"(" + l.sql + " OR " + r.sql + ")"} }

// OpNot is the module-level prefix `!`.
func OpNot(e Expr) Expr { return Expr{"NOT " + e.sql} }

// SequenceContains is Ledger's `Sequence.contains(_: Column)`.
func SequenceContains[E any](seq []E, c Column) Expr {
	parts := make([]string, len(seq))
	for i, x := range seq {
		parts[i] = sqlOf(x)
	}
	return Expr{c.name + " IN (" + strings.Join(parts, ", ") + ")"}
}

// Query is the generic `Query<T>`.
type Query[T any] struct{ sql string }

// QuerySql is `Query.sql`.
func QuerySql[T any](q Query[T]) string { return q.sql }

// QueryLimitOffset is `Query.limit(_:offset:)`.
func QueryLimitOffset[T any](q Query[T], n int, offset *int) Query[T] {
	sql := fmt.Sprintf("%s LIMIT %d", q.sql, n)
	if offset != nil {
		sql += fmt.Sprintf(" OFFSET %d", *offset)
	}
	return Query[T]{sql}
}

var conformances = map[reflect.Type]swiftrt.Conformance{}

// RegisterConformance records a transpiled type's conformance to Ledger's protocols, merging the
// registrations of one type.
func RegisterConformance[T any](c swiftrt.Conformance) {
	t := reflect.TypeFor[T]()
	prev := conformances[t]
	if prev.Witnesses == nil {
		prev.Witnesses = map[string]any{}
	}
	prev.Type = c.Type
	prev.Protocols = append(prev.Protocols, c.Protocols...)
	for k, v := range c.Witnesses {
		prev.Witnesses[k] = v
	}
	conformances[t] = prev
}

func witness[T any](name string) any { return conformances[reflect.TypeFor[T]()].Witnesses[name] }

func tableName[T any]() string {
	if w, ok := witness[T]("tableName").(func() string); ok {
		return w()
	}
	return reflect.TypeFor[T]().Name()
}

// TableFilter is `Table.filter(_:)`, with the conforming type as its type argument.
func TableFilter[T any](e Specific) Query[T] {
	return Query[T]{"SELECT * FROM " + tableName[T]() + " WHERE " + sqlOf(e)}
}

// PersistableInsert is `Persistable.insert(_:)`, taking the record by pointer since it is mutating.
func PersistableInsert[T any](rec *T, b *Book) {
	b.entries = append(b.entries, tableName[T]())
	if didInsert, ok := witness[T]("didInsert(_:)").(func(*T, int64)); ok {
		didInsert(rec, int64(len(b.entries)))
	}
}

// Row is `Row`.
type Row struct{ values map[string]any }

// RowInitMap is `Row(_:)`.
func RowInitMap(values map[string]any) Row { return Row{values} }

// RowSubscriptString is `Row.subscript(_: String) -> V`; V is explicit because it appears only in the result.
func RowSubscriptString[V any](r Row, name string) V { return r.values[name].(V) }

// Arguments is `Arguments`, built from array and dictionary literals.
type Arguments struct{ text string }

// ArgumentsText is `Arguments.text`.
func ArgumentsText(a Arguments) string { return a.text }

// ArgumentsInitArrayLiteral is `Arguments(arrayLiteral:)`.
func ArgumentsInitArrayLiteral(elements ...int) Arguments {
	parts := make([]string, len(elements))
	for i, n := range elements {
		parts[i] = fmt.Sprint(n)
	}
	return Arguments{strings.Join(parts, ",")}
}

// ArgumentsInitDictionaryLiteral is `Arguments(dictionaryLiteral:)`.
func ArgumentsInitDictionaryLiteral(pairs ...swiftrt.KV[string, int]) Arguments {
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = fmt.Sprintf("%s=%d", p.Key, p.Value)
	}
	return Arguments{strings.Join(parts, ",")}
}

// Options is the option set `Options`.
type Options struct{ raw int }

// The members of Options.
var (
	OptionsStrict    = Options{1}
	OptionsTemporary = Options{2}
)

// OptionsRawValue is `Options.rawValue`.
func OptionsRawValue(o Options) int { return o.raw }

// OptionsInitArrayLiteral is `Options(arrayLiteral:)`.
func OptionsInitArrayLiteral(members ...Options) Options {
	var o Options
	for _, m := range members {
		o.raw |= m.raw
	}
	return o
}

// Config is `Config`.
type Config struct{ timeout int }

// ConfigInit is `Config()`.
func ConfigInit() Config { return Config{} }

// ConfigTimeout is `Config.timeout`.
func ConfigTimeout(c Config) int { return c.timeout }

// ConfigSetTimeout sets `Config.timeout`.
func ConfigSetTimeout(c *Config, v int) { c.timeout = v }

// Mode is the enum `Mode`.
type Mode struct {
	safe    bool
	retries int
}

// ModeFast is `Mode.fast`.
var ModeFast = Mode{}

// ModeSafeRetries is `Mode.safe(retries:)`.
func ModeSafeRetries(n int) Mode { return Mode{true, n} }

// ModeRetries is `Mode.retries`.
func ModeRetries(m Mode) int { return m.retries }

// Code is `Code`.
type Code struct{ raw int }

// CodeBusy is `Code.busy`.
var CodeBusy = Code{5}

// Handler is `Handler`.
type Handler struct{ parameterType string }

// HandlerParameterType is `Handler.parameterType`.
func HandlerParameterType(h Handler) string { return h.parameterType }

// HandlerBind is `Handler.bind(_:)`: fn is a swiftrt.Closure, whose Optional flags tell `Book` from `Book?`.
func HandlerBind(fn any) Handler {
	c := fn.(swiftrt.Closure)
	name := map[reflect.Type]string{reflect.TypeFor[*Book](): "Book", reflect.TypeFor[int](): "Int"}[reflect.TypeOf(c.Fn).In(1)]
	if c.Optional[1] {
		name = "Optional<" + name + ">"
	}
	return Handler{name}
}

// HandlerBindIfAny is `Handler.bindIfAny(_:_:)`: fn is nil or a swiftrt.Closure.
func HandlerBindIfAny[A any](fn any, _ swiftrt.TypeT[A]) Handler {
	if fn == nil || reflect.ValueOf(fn).Kind() == reflect.Func && reflect.ValueOf(fn).IsNil() {
		return Handler{"none"}
	}
	return HandlerBind(fn)
}

// FieldName is `FieldName`.
type FieldName struct{ text string }

// FieldNameKind is `FieldName.kind`.
var FieldNameKind = FieldName{"kind"}

// FieldNameTag is `FieldName.tag`.
var FieldNameTag = FieldName{"tag"}

// Fields is `Fields`.
type Fields struct{ values map[string]string }

// FieldsInit is `Fields()`.
func FieldsInit() Fields { return Fields{} }

// FieldsSubscript is the getter of `Fields.subscript(_: FieldName) -> String?`.
func FieldsSubscript(f Fields, name FieldName) *string {
	if v, ok := f.values[name.text]; ok {
		return &v
	}
	return nil
}

// FieldsSetSubscript is the setter of `Fields.subscript(_: FieldName) -> String?`.
func FieldsSetSubscript(f *Fields, name FieldName, v *string) {
	if v == nil {
		delete(f.values, name.text)
		return
	}
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[name.text] = *v
}

// FieldsRendered is `Fields.rendered`.
func FieldsRendered(f Fields) string {
	keys := make([]string, 0, len(f.values))
	for k := range f.values {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + f.values[k]
	}
	return strings.Join(parts, ";")
}

// Message is `Message`.
type Message struct{ fields Fields }

// MessageInit is `Message()`.
func MessageInit() Message { return Message{} }

// MessageFields is `Message.fields`.
func MessageFields(m Message) Fields { return m.fields }

// MessageSetFields sets `Message.fields`.
func MessageSetFields(m *Message, f Fields) { m.fields = f }

// FieldNameText is `FieldName.text`.
func FieldNameText(n FieldName) string { return n.text }

// MessageBody is `MessageBody`.
type MessageBody struct{ text string }

// MessageBodyText is `MessageBody.text`.
func MessageBodyText(b MessageBody) string { return b.text }

// MessageBodyOf is `Message.body`, named apart from the type MessageBody.
func MessageBodyOf(m Message) MessageBody { return MessageBody{FieldsRendered(m.fields)} }
