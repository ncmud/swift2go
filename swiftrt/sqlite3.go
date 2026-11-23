package swiftrt

// The SQLite C module's names that transpiled code calls on a connection handle a Go package hands out,
// such as a GRDB stand-in's `Database.sqliteConnection`. The handle implements the call.

// CMacroSQLITE_LIMIT_VARIABLE_NUMBER is the sqlite3_limit category of the most parameters a statement takes.
const CMacroSQLITE_LIMIT_VARIABLE_NUMBER int32 = 9 //nolint:revive // C name

// SQLiteLimiter is a SQLite connection handle that serves sqlite3_limit.
type SQLiteLimiter interface {
	Sqlite3Limit(id, value int32) int32
}

// CFSqlite3_limit is C `sqlite3_limit(_:_:_:)`: it sets the connection's run-time limit id to value, or
// leaves it for a negative value, and returns the limit before the call.
func CFSqlite3_limit(conn any, id, value int32) int32 { //nolint:revive // C name
	l, ok := conn.(SQLiteLimiter)
	if !ok {
		panic("swiftrt: sqlite3_limit on a connection handle that does not serve it")
	}
	return l.Sqlite3Limit(id, value)
}
