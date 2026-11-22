package services

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"researchhub-backend/config"

	"github.com/google/uuid"
)

type StorageService interface {
	SavePDFFile(fileHeader *multipart.FileHeader, subFolder string) (storedName string, fullPath string, fileSize int64, err error)
	SaveImageFile(fileHeader *multipart.FileHeader, subFolder string) (storedName string, fullPath string, err error)
	ResolveSafePath(storedPath string) (string, error)
}

type LocalStorageService struct {
	BaseDir     string
	MaxBytes    int64
	S3Bucket    string
	S3Endpoint  string
	Driver      string
}

var Storage StorageService

func InitStorageService(cfg *config.AppConfig) {
	baseDir := "./storage"
	maxMB := int64(25)
	driver := "local"
	if cfg != nil {
		if cfg.StorageLocalPath != "" {
			baseDir = cfg.StorageLocalPath
		}
		if cfg.MaxUploadSizeMB > 0 {
			maxMB = cfg.MaxUploadSizeMB
		}
		if cfg.StorageDriver != "" {
			driver = cfg.StorageDriver
		}
	}
	_ = os.MkdirAll(filepath.Join(baseDir, "papers"), 0755)
	_ = os.MkdirAll(filepath.Join(baseDir, "revisions"), 0755)
	_ = os.MkdirAll(filepath.Join(baseDir, "publications"), 0755)
	_ = os.MkdirAll(filepath.Join(baseDir, "certificates"), 0755)
	_ = os.MkdirAll(filepath.Join(baseDir, "logos"), 0755)

	Storage = &LocalStorageService{
		BaseDir:  baseDir,
		MaxBytes: maxMB * 1024 * 1024,
		Driver:   driver,
	}
}

func (s *LocalStorageService) SavePDFFile(fileHeader *multipart.FileHeader, subFolder string) (string, string, int64, error) {
	if fileHeader == nil {
		return "", "", 0, errors.New("no file uploaded")
	}
	if fileHeader.Size > s.MaxBytes {
		return "", "", 0, fmt.Errorf("file size exceeds maximum limit of %d MB", s.MaxBytes/(1024*1024))
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".pdf" {
		return "", "", 0, errors.New("only PDF files (.pdf) are allowed")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// Read first 512 bytes to verify PDF magic bytes & MIME type
	headerBuf := make([]byte, 512)
	n, err := src.Read(headerBuf)
	if err != nil && err != io.EOF {
		return "", "", 0, errors.New("failed to read file header")
	}
	detectedMime := http.DetectContentType(headerBuf[:n])
	hasPDFHeader := pdfHelper.HasPrefixPDF(headerBuf[:n])
	if !hasPDFHeader && detectedMime != "application/pdf" {
		return "", "", 0, errors.New("invalid file content: uploaded file is not a valid PDF document")
	}

	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", "", 0, errors.New("failed to reset file reader")
	}

	safeSubFolder := filepath.Clean(subFolder)
	if strings.Contains(safeSubFolder, "..") {
		return "", "", 0, errors.New("invalid storage subfolder")
	}

	targetDir := filepath.Join(s.BaseDir, safeSubFolder)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", "", 0, fmt.Errorf("failed to create storage directory: %w", err)
	}

	storedName := fmt.Sprintf("%s.pdf", uuid.New().String())
	fullPath := filepath.Join(targetDir, storedName)

	dst, err := os.Create(fullPath)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to save uploaded file: %w", err)
	}

	return storedName, fullPath, written, nil
}

func (s *LocalStorageService) SaveImageFile(fileHeader *multipart.FileHeader, subFolder string) (string, string, error) {
	if fileHeader == nil {
		return "", "", errors.New("no image uploaded")
	}
	if fileHeader.Size > 5*1024*1024 {
		return "", "", errors.New("image size exceeds 5 MB limit")
	}
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp" {
		return "", "", errors.New("only PNG, JPG, JPEG, or WEBP images are allowed")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return "", "", err
	}
	defer src.Close()

	targetDir := filepath.Join(s.BaseDir, filepath.Clean(subFolder))
	_ = os.MkdirAll(targetDir, 0755)

	storedName := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	fullPath := filepath.Join(targetDir, storedName)
	dst, err := os.Create(fullPath)
	if err != nil {
		return "", "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", "", err
	}
	return storedName, fullPath, nil
}

func (s *LocalStorageService) ResolveSafePath(storedPath string) (string, error) {
	if storedPath == "" {
		return "", errors.New("empty file path")
	}
	cleanPath := filepath.Clean(storedPath)
	if strings.Contains(cleanPath, "..") {
		return "", errors.New("path traversal detected")
	}
	info, err := os.Stat(cleanPath)
	if err != nil || info.IsDir() {
		return "", errors.New("file not found on storage")
	}
	return cleanPath, nil
}

var pdfHelper = struct {
	HasPrefixPDF func(b []byte) bool
}{
	HasPrefixPDF: func(b []byte) bool {
		return len(b) >= 4 && string(b[:4]) == "%PDF"
	},
}
