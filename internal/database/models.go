package database

import "time"

// User is the single administrator account for this private gallery.
type User struct {
	ID           uint      `gorm:"primaryKey"`
	Username     string    `gorm:"size:64;not null;uniqueIndex"`
	PasswordHash string    `gorm:"size:255;not null"`
	CreatedAt    time.Time `gorm:"not null"`
	UpdatedAt    time.Time `gorm:"not null"`
}

// UserSession stores only a hash of the browser's opaque session token.
type UserSession struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"not null;index"`
	User      User      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	TokenHash string    `gorm:"size:64;not null;uniqueIndex"`
	ExpiresAt time.Time `gorm:"not null;index"`
	CreatedAt time.Time `gorm:"not null"`
}

// AssetDirectory is a root directory that contains photo albums.
type AssetDirectory struct {
	ID            uint       `gorm:"primaryKey"`
	DirectoryName string     `gorm:"size:255;not null"`
	DirectoryPath string     `gorm:"size:1024;not null;uniqueIndex"`
	AlbumDepth    uint       `gorm:"not null;default:1"`
	ScanStatus    bool       `gorm:"not null;default:false;index"`
	LastScannedAt *time.Time `gorm:"index"`
	ScanError     string     `gorm:"type:text"`
	// AutomationDisabled excludes this directory from filesystem watching,
	// scheduled scans, and user-triggered global scans. A direct scan remains
	// available so an excluded library can still be refreshed explicitly.
	AutomationDisabled bool      `gorm:"not null;default:false"`
	Albums             []Album   `gorm:"foreignKey:DirectoryID"`
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
}

// ScanSchedule stores the single global recurring scan configuration.
type ScanSchedule struct {
	ID         uint       `gorm:"primaryKey"`
	Enabled    bool       `gorm:"not null;default:false"`
	NextScanAt *time.Time `gorm:"index"`
	CreatedAt  time.Time  `gorm:"not null"`
	UpdatedAt  time.Time  `gorm:"not null"`
}

// Album is a directory of photos within an asset directory.
type Album struct {
	PublishedDate string         `gorm:"size:10;not null;default:'';index:idx_albums_gallery,priority:2,sort:desc;index:idx_albums_favorite_gallery,priority:3,sort:desc;index:idx_albums_series_gallery,priority:3,sort:desc"`
	ID            uint           `gorm:"primaryKey;index:idx_albums_gallery,priority:3,sort:desc;index:idx_albums_favorite_gallery,priority:4,sort:desc;index:idx_albums_series_gallery,priority:4,sort:desc"`
	DirectoryID   uint           `gorm:"not null;index"`
	Directory     AssetDirectory `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	SeriesID      *uint          `gorm:"index;index:idx_albums_series_gallery,priority:1"`
	AlbumName     string         `gorm:"size:255;not null"`
	AlbumPath     string         `gorm:"size:1024;not null;uniqueIndex"`
	ScanStatus    bool           `gorm:"not null;default:false;index;index:idx_albums_gallery,priority:1;index:idx_albums_favorite_gallery,priority:1;index:idx_albums_series_gallery,priority:2"`
	Favorite      bool           `gorm:"not null;default:false;index;index:idx_albums_favorite_gallery,priority:2"`
	Photos        []Photo        `gorm:"foreignKey:AlbumID"`
	CreatedAt     time.Time      `gorm:"not null"`
	UpdatedAt     time.Time      `gorm:"not null"`
}

// Person groups albums without attaching metadata to individual media files.
type Person struct {
	ID            uint      `gorm:"primaryKey"`
	Name          string    `gorm:"size:255;not null;index:idx_people_name_lookup"`
	Note          string    `gorm:"size:255;not null;default:''"`
	Aliases       []string  `gorm:"serializer:json;type:json;not null;default:'[]'"`
	Favorite      bool      `gorm:"not null;default:false;index"`
	AvatarPhotoID *uint     `gorm:"index"`
	CreatedAt     time.Time `gorm:"not null"`
	UpdatedAt     time.Time `gorm:"not null"`
}

// PersonAlbum is the many-to-many relationship between people and albums.
type PersonAlbum struct {
	PersonID  uint      `gorm:"primaryKey;index"`
	Person    Person    `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	AlbumID   uint      `gorm:"primaryKey;index"`
	Album     Album     `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	CreatedAt time.Time `gorm:"not null"`
}

// Series groups albums by publisher, collection, or another title-derived label.
type Series struct {
	ID           uint      `gorm:"primaryKey"`
	Name         string    `gorm:"size:255;not null;uniqueIndex"`
	Favorite     bool      `gorm:"not null;default:false;index"`
	CoverPhotoID *uint     `gorm:"index"`
	CreatedAt    time.Time `gorm:"not null"`
	UpdatedAt    time.Time `gorm:"not null"`
}

// Photo stores metadata for an image or video file in an album.
type Photo struct {
	ID            uint      `gorm:"primaryKey"`
	AlbumID       uint      `gorm:"not null;index;index:idx_photos_album_status_name,priority:1"`
	Album         Album     `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	PhotoName     string    `gorm:"size:255;not null;index:idx_photos_album_status_name,priority:3"`
	PhotoPath     string    `gorm:"size:2048;not null;uniqueIndex"`
	ThumbnailPath *string   `gorm:"size:2048"`
	MediaType     string    `gorm:"size:20;not null;default:image;index"`
	Status        string    `gorm:"size:20;not null;default:active;index;index:idx_photos_album_status_name,priority:2"`
	Favorite      bool      `gorm:"not null;default:false;index"`
	FileSize      int64     `gorm:"not null;default:0"`
	HeightPX      *uint     `gorm:"column:height_px"`
	WidthPX       *uint     `gorm:"column:width_px"`
	DurationMS    *int64    `gorm:"column:duration_ms"`
	MIMEType      string    `gorm:"column:mime_type;size:100;not null;index"`
	ModifiedAt    time.Time `gorm:"not null"`
	LastSeenAt    time.Time `gorm:"not null;index"`
	CreatedAt     time.Time `gorm:"not null"`
	UpdatedAt     time.Time `gorm:"not null"`
}
