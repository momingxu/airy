package database

import (
	"path/filepath"
	"testing"

	"airy/internal/config"
)

func TestMigrateLegacyPersonAliases(t *testing.T) {
	db, err := Open(config.DatabaseConfig{Driver: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "aliases.db")})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	person := Person{Name: "Alice", Aliases: []string{"Existing"}}
	other := Person{Name: "Alice"}
	for _, p := range []*Person{&person, &other} {
		if err := db.Create(p).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`CREATE TABLE person_aliases (
		id integer PRIMARY KEY, person_id integer, alias text, normalized_alias text
	)`).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		personID          uint
		alias, normalized string
	}{
		{person.ID, " Ally ", "ally"},
		{person.ID, "ALLY", "ally"},
		{person.ID, "Alice", "alice"},
		{other.ID, "Ally", "ally"},
	} {
		if err := db.Exec("INSERT INTO person_aliases(person_id, alias, normalized_alias) VALUES (?, ?, ?)", row.personID, row.alias, row.normalized).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("person_aliases") {
		t.Fatal("legacy aliases table remains")
	}
	if err := db.First(&person, person.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(person.Aliases) != 2 || person.Aliases[0] != "Existing" || person.Aliases[1] != "Ally" {
		t.Fatalf("unexpected migrated aliases: %#v", person.Aliases)
	}
	if err := db.First(&other, other.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(other.Aliases) != 1 || other.Aliases[0] != "Ally" {
		t.Fatalf("shared aliases must remain separate: %#v", other.Aliases)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
}
