package item

import (
	"reflect"
	"testing"
	"time"
)

func TestReadAndWriteDatabase(t *testing.T) {
	vaultItem := Item{
		Overview: Overview{Category: CategoryDatabase, Username: "app"},
		Details: Details{Password: "db-password", Fields: []Field{
			{Name: "SID", Value: "ORCL"},
			{Name: "database", Value: " app "},
			{Name: "host", Value: " db.example.com:5432 "},
			{Name: "ENGINE", Value: " Postgres "},
			{Name: "options", Value: "?sslmode=require"},
		}},
	}
	database, otherFields := ReadDatabase(&vaultItem)
	if database.Engine != " Postgres " || database.Name != " app " || !reflect.DeepEqual(otherFields, []Field{{Name: "SID", Value: "ORCL"}}) {
		t.Errorf("database = %+v, other fields %+v", database, otherFields)
	}
	NormalizeDatabase(&vaultItem)
	wantFields := []Field{
		{Name: FieldEngine, Value: "postgresql"},
		{Name: FieldHost, Value: "db.example.com"},
		{Name: FieldPort, Value: "5432"},
		{Name: FieldDatabaseName, Value: "app"},
		{Name: FieldOptions, Value: "sslmode=require"},
		{Name: "SID", Value: "ORCL"},
	}
	if !reflect.DeepEqual(vaultItem.Details.Fields, wantFields) {
		t.Errorf("normalized fields =\n%+v\nwant\n%+v", vaultItem.Details.Fields, wantFields)
	}
	if vaultItem.Overview.Username != "app" || vaultItem.Details.Password != "db-password" {
		t.Errorf("normalizing changed the login data: %+v %+v", vaultItem.Overview, vaultItem.Details)
	}
	if len(ListDatabaseFields()) != 5 || len(ListDatabaseEngines()) != 7 {
		t.Errorf("database fields = %d, engines = %d", len(ListDatabaseFields()), len(ListDatabaseEngines()))
	}
	login := Item{Details: Details{Fields: []Field{{Name: "Engine", Value: "Postgres"}}}}
	NormalizeDatabase(&login)
	if login.Details.Fields[0].Value != "Postgres" {
		t.Errorf("NormalizeDatabase changed a login: %+v", login.Details.Fields)
	}
}

func TestDatabaseEngines(t *testing.T) {
	for typed, want := range map[string]string{
		"postgresql": "postgresql", "PostgreSQL": "postgresql", "postgres": "postgresql", "pgsql": "postgresql",
		"MySQL": "mysql", "mariadb": "mariadb", "mssql": "sqlserver", "SQL Server": "sqlserver", "sqlserver": "sqlserver",
		"oracle": "oracle", "mongo": "mongodb", "Redis": "redis", " db2 ": "db2", "": "",
	} {
		if got := (Database{Engine: typed}).Normalize().Engine; got != want {
			t.Errorf("engine %q normalizes to %q, want %q", typed, got, want)
		}
	}
	if got := (Database{Engine: "sqlserver"}).EngineName(); got != "SQL Server" {
		t.Errorf("EngineName = %q", got)
	}
	if got := (Database{Engine: "DB2"}).EngineName(); got != "DB2" {
		t.Errorf("EngineName of another engine = %q", got)
	}
}

func TestDatabaseSummaryAndPortWarning(t *testing.T) {
	for _, test := range []struct {
		database Database
		want     string
	}{
		{Database{Engine: "postgresql", Host: "db.example.com", Port: "5432", Name: "app"}, "PostgreSQL · db.example.com:5432/app"},
		{Database{Engine: "redis", Host: "2001:db8::1"}, "Redis · [2001:db8::1]"},
		{Database{Engine: "db2", Name: "app"}, "db2 · app"},
		{Database{}, ""},
	} {
		if got := test.database.Summary(); got != test.want {
			t.Errorf("Summary(%+v) = %q, want %q", test.database, got, test.want)
		}
	}
	if (Database{Port: "70000"}).PortWarning() == "" || (Database{Port: "5432"}).PortWarning() != "" {
		t.Error("the port warning is wrong")
	}
}

func TestDatabaseConnectionString(t *testing.T) {
	for _, test := range []struct {
		database           Database
		username, password string
		want               string
	}{
		{Database{Engine: "postgresql", Host: "db.example.com", Port: "5432", Name: "app", Options: "sslmode=require"}, "app", "p@ss:w/rd",
			"postgresql://app:p%40ss%3Aw%2Frd@db.example.com:5432/app?sslmode=require"},
		{Database{Engine: "mariadb", Host: "db.example.com", Name: "shop"}, "root", "", "mysql://root@db.example.com/shop"},
		{Database{Engine: "sqlserver", Host: "sql.example.com", Port: "1433", Name: "Sales DB", Options: "encrypt=true"}, "sa", "secret",
			"sqlserver://sa:secret@sql.example.com:1433?database=Sales+DB&encrypt=true"},
		{Database{Engine: "mongodb", Host: "2001:db8::1", Port: "27017"}, "", "", "mongodb://[2001:db8::1]:27017"},
		{Database{Engine: "redis", Host: "cache.example.com", Port: "6379", Name: "0"}, "", "secret", "redis://:secret@cache.example.com:6379/0"},
		{Database{Engine: "db2", Host: "db.example.com"}, "app", "secret", ""},
		{Database{Engine: "postgresql"}, "app", "secret", ""},
	} {
		if got := test.database.ConnectionString(test.username, test.password); got != test.want {
			t.Errorf("ConnectionString(%+v) = %q, want %q", test.database, got, test.want)
		}
	}
}

func TestLookupConnectionString(t *testing.T) {
	names := FieldNames{"connection string": FieldConnectionString}
	database := Item{Overview: Overview{Category: CategoryDatabase, Username: "app"}, Details: Details{Password: "secret"}}
	WriteDatabase(&database, Database{Engine: "Postgres", Host: "db.example.com:5432", Name: "app"}, nil)
	if value, found := LookupField(&database, "Connection String", names, time.Now()); !found || value != "postgresql://app:secret@db.example.com:5432/app" {
		t.Errorf("connection string = %q, %v", value, found)
	}
	// A login imported with a custom field of that name keeps it.
	login := Item{Details: Details{Fields: []Field{{Name: "Connection string", Value: "jdbc:db2://db.example.com:50000/app"}}}}
	if value, found := LookupField(&login, "connection string", names, time.Now()); !found || value != "jdbc:db2://db.example.com:50000/app" {
		t.Errorf("login's connection string = %q, %v", value, found)
	}
	if login.ConnectionString() != "" {
		t.Error("a login builds a connection string")
	}
}
