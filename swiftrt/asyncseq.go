package swiftrt

// AsyncSequence is implemented by runtime types that model a Swift `AsyncSequence`: the returned
// function is the iterator's `next()`, which waits for an element and returns nil at the end.
type AsyncSequence[T any] interface {
	AsyncNextFunc() func() *T
}

// AsyncIter is the iterator a `for await` loop pulls from. Sequences that are not asynchronous are
// walked in order.
func AsyncIter[T any](s any) func() *T {
	if a, ok := s.(AsyncSequence[T]); ok {
		return a.AsyncNextFunc()
	}
	xs := Iterate[T](s)
	i := 0
	return func() *T {
		if i >= len(xs) {
			return nil
		}
		v := xs[i]
		i++
		return &v
	}
}

// AsyncNextFunc makes AsyncStream an AsyncSequence.
func (s AsyncStream[T]) AsyncNextFunc() func() *T {
	it := AsyncStreamMakeAsyncIterator(s)
	return func() *T { return AsyncStreamIteratorNext(&it) }
}

// AsyncNextFunc makes WebSocketInboundMessageStream an AsyncSequence.
func (s WebSocketInboundMessageStream) AsyncNextFunc() func() *WebSocketMessage {
	it := WebSocketInboundMessageStreamMakeAsyncIterator(s)
	return func() *WebSocketMessage { return WebSocketInboundMessageStreamAsyncIteratorNext(&it) }
}
