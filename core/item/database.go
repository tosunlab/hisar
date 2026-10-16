package item

import (
	"net"
	"net/url"
	"slices"
	"strings"
)

// CategoryDatabase items keep their username and password as the item's own login data, as servers do, and their engine,
// host, port, database name and connection options as custom fields with the names below. Their connection string is
// never stored: ConnectionString builds it from those values, so it always carries the current password.
const CategoryDatabase = "database"

// The custom field names of a database's values; its host and port are FieldHost and FieldPort, as a server's.
const (
	FieldEngine       = "Engine"
	FieldDatabaseName = "Database"
	FieldOptions      = "Options"
)

// Database is what a database item holds besides its login data. Engine is the ID of one of ListDatabaseEngines, or any
// other engine as typed; Options are connection options such as sslmode=require.
type Database struct {
	Engine  string
	Host    string
	Port    string
	Name    string
	Options string
}

// DatabaseField is one of a database's values.
type DatabaseField = NamedField[Database]

// ListDatabaseFields returns a database's fields in the order they are stored and asked for.
func ListDatabaseFields() []DatabaseField { return slices.Clone(databaseFields) }

var databaseFields = []DatabaseField{
	{FieldEngine, false, func(database *Database) *string { return &database.Engine }},
	{FieldHost, false, func(database *Database) *string { return &database.Host }},
	{FieldPort, false, func(database *Database) *string { return &database.Port }},
	{FieldDatabaseName, false, func(database *Database) *string { return &database.Name }},
	{FieldOptions, false, func(database *Database) *string { return &database.Options }},
}

// DatabaseEngine is an engine Hisar builds connection strings for; Scheme starts them.
type DatabaseEngine struct {
	ID     string
	Name   string
	Scheme string
}

var databaseEngines = []DatabaseEngine{
	{"postgresql", "PostgreSQL", "postgresql"},
	{"mysql", "MySQL", "mysql"},
	{"mariadb", "MariaDB", "mysql"},
	{"sqlserver", "SQL Server", "sqlserver"},
	{"oracle", "Oracle", "oracle"},
	{"mongodb", "MongoDB", "mongodb"},
	{"redis", "Redis", "redis"},
}

// databaseEngineAliases maps other names of the engines, lower-cased, to their IDs; 1Password calls SQL Server "mssql".
var databaseEngineAliases = map[string]string{
	"postgres": "postgresql", "pgsql": "postgresql", "pg": "postgresql",
	"mssql": "sqlserver", "sql server": "sqlserver", "microsoft sql server": "sqlserver",
	"mongo": "mongodb",
}

// ListDatabaseEngines returns the engines the editors offer, in the order they offer them.
func ListDatabaseEngines() []DatabaseEngine { return slices.Clone(databaseEngines) }

// findDatabaseEngine returns the engine an ID, name or alias names, ignoring case.
func findDatabaseEngine(text string) (DatabaseEngine, bool) {
	key := strings.ToLower(strings.TrimSpace(text))
	if id, isAlias := databaseEngineAliases[key]; isAlias {
		key = id
	}
	for _, engine := range databaseEngines {
		if key == engine.ID || key == strings.ToLower(engine.Name) {
			return engine, true
		}
	}
	return DatabaseEngine{}, false
}

// ReadDatabase returns the database's values and the item's other custom fields, matching names ignoring case.
func ReadDatabase(vaultItem *Item) (Database, []Field) { return readRecord(vaultItem, databaseFields) }

// WriteDatabase stores database as the item's first custom fields, in a fixed order, followed by otherFields; an empty
// value is left out.
func WriteDatabase(vaultItem *Item, database Database, otherFields []Field) {
	writeRecord(vaultItem, database, databaseFields, otherFields)
}

// NormalizeDatabase trims a database item's values and puts its fields in order; other items are left alone.
func NormalizeDatabase(vaultItem *Item) {
	if !vaultItem.IsCategory(CategoryDatabase) {
		return
	}
	database, otherFields := ReadDatabase(vaultItem)
	WriteDatabase(vaultItem, database.Normalize(), otherFields)
}

// Normalize trims the database's values, stores a known engine by its ID (so "Postgres" and "mssql" become postgresql and
// sqlserver) and splits a host typed with a scheme or port, as a server's. Another engine stays as typed.
func (database Database) Normalize() Database {
	database.Engine = strings.TrimSpace(database.Engine)
	if engine, isKnown := findDatabaseEngine(database.Engine); isKnown {
		database.Engine = engine.ID
	}
	database.Host, database.Port = splitHostAndPort(strings.TrimSpace(database.Host), strings.TrimSpace(database.Port))
	database.Name = strings.TrimSpace(database.Name)
	database.Options = strings.TrimPrefix(strings.TrimSpace(database.Options), "?")
	return database
}

// EngineName is the engine's name, such as "PostgreSQL", or the engine as typed when Hisar does not know it.
func (database Database) EngineName() string {
	if engine, isKnown := findDatabaseEngine(database.Engine); isKnown {
		return engine.Name
	}
	return strings.TrimSpace(database.Engine)
}

// PortWarning is what the editors show under a port that is not a number from 1 to 65535, as for a server.
func (database Database) PortWarning() string { return describePort(database.Port) }

// address is the host and port, such as "db.example.com:5432" or "[2001:db8::1]:5432", or the host alone.
func (database Database) address() string {
	switch {
	case database.Host == "":
		return ""
	case database.Port != "":
		return net.JoinHostPort(database.Host, database.Port)
	case strings.Contains(database.Host, ":"):
		return "[" + database.Host + "]"
	}
	return database.Host
}

// Summary is the database's line in lists: its engine, then its address and database name, such as
// "PostgreSQL · db.example.com:5432/app".
func (database Database) Summary() string {
	location := database.address()
	if database.Name != "" {
		location = strings.TrimPrefix(location+"/"+database.Name, "/")
	}
	return joinNonEmpty(" · ", database.EngineName(), location)
}

// ConnectionString builds the URL clients connect with, such as postgresql://app:secret@db.example.com:5432/app?sslmode=require,
// escaping the username and password. SQL Server names the database in the query. It is "" for an engine Hisar does not
// know or without a host.
func (database Database) ConnectionString(username, password string) string {
	engine, isKnown := findDatabaseEngine(database.Engine)
	if !isKnown || database.Host == "" {
		return ""
	}
	connection := url.URL{Scheme: engine.Scheme, Host: database.address()}
	switch {
	case password != "":
		connection.User = url.UserPassword(username, password)
	case username != "":
		connection.User = url.User(username)
	}
	query := []string{}
	if database.Name != "" && engine.ID == "sqlserver" {
		query = append(query, "database="+url.QueryEscape(database.Name))
	} else if database.Name != "" {
		connection.Path = "/" + database.Name
	}
	if database.Options != "" {
		query = append(query, database.Options)
	}
	connection.RawQuery = strings.Join(query, "&")
	return connection.String()
}

// ConnectionString builds a database item's connection string from its values and login data; it is "" for other items.
func (vaultItem *Item) ConnectionString() string {
	if !vaultItem.IsCategory(CategoryDatabase) {
		return ""
	}
	database, _ := ReadDatabase(vaultItem)
	return database.Normalize().ConnectionString(vaultItem.Overview.Username, vaultItem.Details.Password)
}
