package database

import (
	"strings"

	"gorm.io/gorm"
)

// CleanPersonAliases keeps aliases unique within each model, while allowing
// different models to share the same name or alias.
func CleanPersonAliases(values []string, primaryName string) []string {
	normalize := func(value string) string {
		return strings.ToLower(strings.Join(strings.Fields(value), " "))
	}
	primary := normalize(primaryName)
	seen := make(map[string]struct{}, len(values))
	aliases := make([]string, 0, len(values))
	for _, value := range values {
		alias := strings.Join(strings.Fields(value), " ")
		key := normalize(alias)
		if key == "" || key == primary {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		aliases = append(aliases, alias)
	}
	return aliases
}

func migrateLegacyPersonAliases(db *gorm.DB) error {
	if !db.Migrator().HasTable("person_aliases") {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var batch []Person
		if err := tx.FindInBatches(&batch, 100, func(batchTx *gorm.DB, _ int) error {
			for _, person := range batch {
				var aliases []string
				if err := batchTx.Table("person_aliases").Where("person_id = ?", person.ID).
					Order("normalized_alias ASC").Order("id ASC").Pluck("alias", &aliases).Error; err != nil {
					return err
				}
				person.Aliases = CleanPersonAliases(append(person.Aliases, aliases...), person.Name)
				if err := batchTx.Model(&person).Select("Aliases").Updates(&person).Error; err != nil {
					return err
				}
			}
			return nil
		}).Error; err != nil {
			return err
		}
		return tx.Migrator().DropTable("person_aliases")
	})
}
