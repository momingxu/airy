package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"airy/internal/database"
	"gorm.io/gorm"
)

const csvClear = "[清空]"

type csvEntities struct {
	series       map[uint]database.Series
	seriesByName map[string]uint
	people       map[uint]database.Person
	peopleByName map[string][]uint
	links        map[uint][]uint
}

func loadCSVEntities(db *gorm.DB, albumIDs []uint) (*csvEntities, error) {
	e := &csvEntities{series: map[uint]database.Series{}, seriesByName: map[string]uint{}, people: map[uint]database.Person{}, peopleByName: map[string][]uint{}, links: map[uint][]uint{}}
	var series []database.Series
	var people []database.Person
	if err := db.Select("id", "name").Find(&series).Error; err != nil {
		return nil, err
	}
	if err := db.Select("id", "name").Order("id ASC").Find(&people).Error; err != nil {
		return nil, err
	}
	for _, s := range series {
		e.series[s.ID] = s
		e.seriesByName[s.Name] = s.ID
	}
	for _, p := range people {
		e.people[p.ID] = p
		e.peopleByName[p.Name] = append(e.peopleByName[p.Name], p.ID)
	}
	for start := 0; start < len(albumIDs); start += 200 {
		var links []database.PersonAlbum
		if err := db.Where("album_id IN ?", albumIDs[start:min(start+200, len(albumIDs))]).Order("person_id ASC").Find(&links).Error; err != nil {
			return nil, err
		}
		for _, link := range links {
			e.links[link.AlbumID] = append(e.links[link.AlbumID], link.PersonID)
		}
	}
	for id, links := range e.links {
		sort.Slice(links, func(i, j int) bool { return links[i] < links[j] })
		e.links[id] = slices.Compact(links)
	}
	return e, nil
}
func (e *csvEntities) seriesName(id *uint) string {
	if id == nil {
		return ""
	}
	return e.series[*id].Name
}
func (e *csvEntities) personNames(ids []uint) []string {
	names := []string{}
	for _, id := range ids {
		names = append(names, e.people[id].Name)
	}
	return names
}
func csvPeopleText(names []string) string {
	for _, name := range names {
		if strings.ContainsAny(name, ";；\n\r") || strings.HasPrefix(name, "[") || name != strings.TrimSpace(name) {
			value, _ := json.Marshal(names)
			return string(value)
		}
	}
	return strings.Join(names, ";")
}
func parseCSVPeople(value string) ([]string, error) {
	var names []string
	if strings.HasPrefix(value, "[") {
		if err := json.Unmarshal([]byte(value), &names); err != nil {
			return nil, errors.New("模特列请用分号分隔名称，或使用 JSON 名称数组")
		}
	} else {
		names = strings.FieldsFunc(value, func(r rune) bool { return r == ';' || r == '；' })
	}
	unique := []string{}
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, errors.New("模特名称不能为空")
		}
		if !seen[name] {
			unique = append(unique, name)
			seen[name] = true
		}
	}
	if len(unique) == 0 {
		return nil, errors.New("模特名称不能为空；解除关联请填写 [清空]")
	}
	if len(unique) > 100 {
		return nil, errors.New("每个相册最多导入 100 个模特")
	}
	return unique, nil
}
func validCSVEntityName(name string) bool {
	return strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= 255 && !strings.ContainsRune(name, 0)
}
func equalSeriesID(a, b *uint) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func (e *csvEntities) fillRow(row *albumCSVRow, album database.Album, record []string) []string {
	row.OldSeriesID = album.SeriesID
	row.SeriesID = album.SeriesID
	row.OldSeries = e.seriesName(album.SeriesID)
	row.Series = row.OldSeries
	row.OldPersonIDs = append([]uint{}, e.links[album.ID]...)
	row.PersonIDs = append([]uint{}, row.OldPersonIDs...)
	row.OldPeople = e.personNames(row.OldPersonIDs)
	row.People = append([]string{}, row.OldPeople...)
	row.NewPeople = []string{}
	if len(record) < 5 {
		return nil
	}
	var issues []string
	series := strings.TrimSpace(record[3])
	people := strings.TrimSpace(record[4])
	if record[3] == spreadsheetTitle(row.OldSeries) {
		series = row.OldSeries
	}
	if series != "" && series != row.OldSeries {
		row.UpdateSeries = true
		row.SeriesID = nil
		row.Series = series
		if series == csvClear {
			row.Series = ""
		} else if !validCSVEntityName(series) {
			issues = append(issues, "系列名称不能为空或超过 255 字符")
		} else if id, ok := e.seriesByName[series]; ok {
			row.SeriesID = &id
		} else {
			row.CreateSeries = true
		}
	}
	if people != "" && record[4] != spreadsheetTitle(csvPeopleText(row.OldPeople)) {
		row.UpdatePeople = true
		row.PersonIDs = []uint{}
		row.People = []string{}
		if people != csvClear {
			names, err := parseCSVPeople(people)
			if err != nil {
				issues = append(issues, err.Error())
			} else {
				row.People = names
				for _, name := range names {
					if !validCSVEntityName(name) {
						issues = append(issues, "模特名称不能为空或超过 255 字符")
						continue
					}
					ids := e.peopleByName[name]
					if len(ids) > 1 {
						issues = append(issues, fmt.Sprintf("存在多个同名模特 %q，请先在模特页区分名称", name))
					} else if len(ids) == 1 {
						row.PersonIDs = append(row.PersonIDs, ids[0])
					} else {
						row.NewPeople = append(row.NewPeople, name)
					}
				}
			}
		}
		slices.Sort(row.PersonIDs)
	}
	return issues
}

func applyCSVEntities(tx *gorm.DB, rows []albumCSVRow) error {
	createdSeries := map[string]uint{}
	createdPeople := map[string]uint{}
	var personRefresh, seriesRefresh []uint
	for _, row := range rows {
		if !row.Changed {
			continue
		}
		if row.UpdateSeries {
			seriesID := row.SeriesID
			if row.CreateSeries {
				id, ok := createdSeries[row.Series]
				if !ok {
					item := database.Series{Name: row.Series}
					if err := tx.Create(&item).Error; err != nil {
						return err
					}
					id = item.ID
					createdSeries[row.Series] = id
				}
				seriesID = &id
			}
			if err := tx.Model(&database.Album{}).Where("id = ?", row.ID).UpdateColumn("series_id", seriesID).Error; err != nil {
				return err
			}
			if row.OldSeriesID != nil {
				seriesRefresh = append(seriesRefresh, *row.OldSeriesID)
			}
			if seriesID != nil {
				seriesRefresh = append(seriesRefresh, *seriesID)
			}
		}
		if row.UpdatePeople {
			ids := append([]uint{}, row.PersonIDs...)
			for _, name := range row.NewPeople {
				id, ok := createdPeople[name]
				if !ok {
					person := database.Person{Name: name, Aliases: []string{}}
					if err := tx.Create(&person).Error; err != nil {
						return err
					}
					id = person.ID
					createdPeople[name] = id
				}
				ids = append(ids, id)
			}
			if err := tx.Where("album_id = ?", row.ID).Delete(&database.PersonAlbum{}).Error; err != nil {
				return err
			}
			for _, id := range ids {
				if err := tx.Create(&database.PersonAlbum{AlbumID: row.ID, PersonID: id}).Error; err != nil {
					return err
				}
			}
			personRefresh = append(personRefresh, row.OldPersonIDs...)
			personRefresh = append(personRefresh, ids...)
		}
	}
	if err := database.RefreshPersonAvatarPhotoIDs(tx, uniqueUintIDs(personRefresh)); err != nil {
		return err
	}
	return database.RefreshSeriesCoverPhotoIDs(tx, uniqueUintIDs(seriesRefresh))
}
