package media

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"airy/internal/database"

	"gorm.io/gorm"
)

var (
	ErrUnsafePath       = errors.New("media path is outside its configured root")
	ErrDerivativeAbsent = errors.New("media derivative has not been generated")
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) OpenOriginal(ctx context.Context, id uint) (database.Photo, *os.File, fs.FileInfo, error) {
	photo, err := s.find(ctx, id)
	if err != nil {
		return database.Photo{}, nil, nil, err
	}
	return openValidated(photo, photo.PhotoPath)
}

func (s *Service) OpenThumbnail(ctx context.Context, id uint) (database.Photo, *os.File, fs.FileInfo, error) {
	photo, err := s.find(ctx, id)
	if err != nil {
		return database.Photo{}, nil, nil, err
	}
	if photo.ThumbnailPath == nil || *photo.ThumbnailPath == "" {
		return database.Photo{}, nil, nil, ErrDerivativeAbsent
	}
	return openValidated(photo, *photo.ThumbnailPath)
}

func (s *Service) find(ctx context.Context, id uint) (database.Photo, error) {
	if s == nil || s.db == nil {
		return database.Photo{}, errors.New("media service is unavailable")
	}
	var photo database.Photo
	err := s.db.WithContext(ctx).
		Preload("Album.Directory").
		Where("id = ? AND status = ?", id, "active").
		First(&photo).Error
	return photo, err
}

func openValidated(photo database.Photo, requestedPath string) (database.Photo, *os.File, fs.FileInfo, error) {
	resolvedPath, err := filepath.EvalSymlinks(requestedPath)
	if err != nil {
		return database.Photo{}, nil, nil, err
	}
	root, err := filepath.EvalSymlinks(photo.Album.Directory.DirectoryPath)
	if err != nil {
		return database.Photo{}, nil, nil, err
	}
	relative, err := filepath.Rel(root, resolvedPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return database.Photo{}, nil, nil, ErrUnsafePath
	}

	file, err := os.Open(resolvedPath)
	if err != nil {
		return database.Photo{}, nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return database.Photo{}, nil, nil, err
	}
	return photo, file, info, nil
}
