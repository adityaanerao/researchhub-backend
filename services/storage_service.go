package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"researchhub-backend/config"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

// StorageService defines a unified abstraction for local filesystem and private AWS S3 storage.
type StorageService interface {
	SavePDFFile(fileHeader *multipart.FileHeader, subFolder string) (storedName string, fullPath string, fileSize int64, err error)
	SaveImageFile(fileHeader *multipart.FileHeader, subFolder string) (storedName string, fullPath string, err error)
	SaveGeneratedFile(localPath string, subFolder string, contentType string) (storedName string, objectKey string, err error)
	GetFile(storedPath string) (io.ReadCloser, string, int64, error)
	ReadFile(storedPath string) (io.ReadCloser, string, int64, error)
	DeleteFile(storedPath string) error
	ResolveSafePath(storedPath string) (string, error)
}

// S3ClientAPI is an injectable interface for AWS S3 operations (enables unit testing with mocks).
type S3ClientAPI interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

type LocalStorageService struct {
	BaseDir  string
	MaxBytes int64
	Driver   string
}

type S3StorageService struct {
	Client   S3ClientAPI
	Bucket   string
	Region   string
	BaseDir  string
	MaxBytes int64
}

var Storage StorageService

// InitStorageService initializes either LocalStorageService or S3StorageService based on config.
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
			driver = strings.ToLower(strings.TrimSpace(cfg.StorageDriver))
		}
	}

	_ = os.MkdirAll(filepath.Join(baseDir, "papers"), 0755)
	_ = os.MkdirAll(filepath.Join(baseDir, "revisions"), 0755)
	_ = os.MkdirAll(filepath.Join(baseDir, "publications"), 0755)
	_ = os.MkdirAll(filepath.Join(baseDir, "certificates"), 0755)
	_ = os.MkdirAll(filepath.Join(baseDir, "logos"), 0755)
	_ = os.MkdirAll(filepath.Join(baseDir, "bills"), 0755)

	if driver == "s3" {
		if cfg == nil || strings.TrimSpace(cfg.S3Region) == "" || strings.TrimSpace(cfg.S3Bucket) == "" {
			log.Fatalf("STORAGE_DRIVER=s3 requires S3_REGION and S3_BUCKET to be configured")
		}
		awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(cfg.S3Region))
		if err != nil {
			log.Fatalf("Failed to load AWS SDK configuration: %v", err)
		}
		s3Client := s3.NewFromConfig(awsCfg)
		Storage = &S3StorageService{
			Client:   s3Client,
			Bucket:   strings.TrimSpace(cfg.S3Bucket),
			Region:   strings.TrimSpace(cfg.S3Region),
			BaseDir:  baseDir,
			MaxBytes: maxMB * 1024 * 1024,
		}
		log.Printf("StorageService initialized with AWS S3 (bucket=%s, region=%s)", cfg.S3Bucket, cfg.S3Region)
		return
	}

	Storage = &LocalStorageService{
		BaseDir:  baseDir,
		MaxBytes: maxMB * 1024 * 1024,
		Driver:   "local",
	}
	log.Printf("StorageService initialized with local storage (dir=%s)", baseDir)
}

func validatePDFHeader(fileHeader *multipart.FileHeader, maxBytes int64) (multipart.File, error) {
	if fileHeader == nil {
		return nil, errors.New("no file uploaded")
	}
	if fileHeader.Size > maxBytes {
		return nil, fmt.Errorf("file size exceeds maximum limit of %d MB", maxBytes/(1024*1024))
	}
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".pdf" {
		return nil, errors.New("only PDF files (.pdf) are allowed")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %w", err)
	}

	headerBuf := make([]byte, 512)
	n, err := src.Read(headerBuf)
	if err != nil && err != io.EOF {
		_ = src.Close()
		return nil, errors.New("failed to read file header")
	}
	detectedMime := http.DetectContentType(headerBuf[:n])
	hasPDFHeader := pdfHelper.HasPrefixPDF(headerBuf[:n])
	if !hasPDFHeader && detectedMime != "application/pdf" {
		_ = src.Close()
		return nil, errors.New("invalid file content: uploaded file is not a valid PDF document")
	}

	if _, err := src.Seek(0, io.SeekStart); err != nil {
		_ = src.Close()
		return nil, errors.New("failed to reset file reader")
	}
	return src, nil
}

func validateImageHeader(fileHeader *multipart.FileHeader) (multipart.File, string, string, error) {
	if fileHeader == nil {
		return nil, "", "", errors.New("no image uploaded")
	}
	if fileHeader.Size > 5*1024*1024 {
		return nil, "", "", errors.New("image size exceeds 5 MB limit")
	}
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp" {
		return nil, "", "", errors.New("only PNG, JPG, JPEG, or WEBP images are allowed")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return nil, "", "", err
	}

	headerBuf := make([]byte, 512)
	n, err := src.Read(headerBuf)
	if err != nil && err != io.EOF {
		_ = src.Close()
		return nil, "", "", errors.New("failed to read image header")
	}
	detectedMime := http.DetectContentType(headerBuf[:n])
	if detectedMime != "image/png" && detectedMime != "image/jpeg" && detectedMime != "image/webp" {
		_ = src.Close()
		return nil, "", "", errors.New("invalid image content: only PNG, JPG, JPEG, or WEBP images are allowed")
	}

	if _, err := src.Seek(0, io.SeekStart); err != nil {
		_ = src.Close()
		return nil, "", "", errors.New("failed to reset image reader")
	}
	return src, ext, detectedMime, nil
}

func sanitizeSubFolder(subFolder string) (string, error) {
	clean := path.Clean(strings.ReplaceAll(strings.TrimSpace(subFolder), "\\", "/"))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "/") || strings.Contains(clean, "..") {
		return "", errors.New("invalid storage subfolder")
	}
	return clean, nil
}

func sanitizeObjectKey(key string) (string, error) {
	clean := path.Clean(strings.ReplaceAll(strings.TrimSpace(key), "\\", "/"))
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." || strings.Contains(clean, "..") {
		return "", errors.New("invalid object key")
	}
	return clean, nil
}

// ---------------- LocalStorageService ----------------

func (s *LocalStorageService) SavePDFFile(fileHeader *multipart.FileHeader, subFolder string) (string, string, int64, error) {
	src, err := validatePDFHeader(fileHeader, s.MaxBytes)
	if err != nil {
		return "", "", 0, err
	}
	defer src.Close()

	safeSubFolder, err := sanitizeSubFolder(subFolder)
	if err != nil {
		return "", "", 0, err
	}

	targetDir := filepath.Join(s.BaseDir, filepath.FromSlash(safeSubFolder))
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
	src, ext, _, err := validateImageHeader(fileHeader)
	if err != nil {
		return "", "", err
	}
	defer src.Close()

	safeSubFolder, err := sanitizeSubFolder(subFolder)
	if err != nil {
		return "", "", err
	}

	targetDir := filepath.Join(s.BaseDir, filepath.FromSlash(safeSubFolder))
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

func (s *LocalStorageService) SaveGeneratedFile(localPath string, subFolder string, contentType string) (string, string, error) {
	safeSubFolder, err := sanitizeSubFolder(subFolder)
	if err != nil {
		return "", "", err
	}
	targetDir := filepath.Join(s.BaseDir, filepath.FromSlash(safeSubFolder))
	_ = os.MkdirAll(targetDir, 0755)

	absSrc, _ := filepath.Abs(localPath)
	absTargetDir, _ := filepath.Abs(targetDir)
	if strings.HasPrefix(absSrc, absTargetDir+string(os.PathSeparator)) {
		return filepath.Base(localPath), localPath, nil
	}

	storedName := filepath.Base(localPath)
	destPath := filepath.Join(targetDir, storedName)
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return "", "", err
	}
	return storedName, destPath, nil
}

func (s *LocalStorageService) GetFile(storedPath string) (io.ReadCloser, string, int64, error) {
	safePath, err := s.ResolveSafePath(storedPath)
	if err != nil {
		return nil, "", 0, err
	}
	f, err := os.Open(safePath)
	if err != nil {
		return nil, "", 0, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, "", 0, err
	}
	ct := "application/octet-stream"
	if strings.HasSuffix(strings.ToLower(safePath), ".pdf") {
		ct = "application/pdf"
	} else if strings.HasSuffix(strings.ToLower(safePath), ".png") {
		ct = "image/png"
	} else if strings.HasSuffix(strings.ToLower(safePath), ".jpg") || strings.HasSuffix(strings.ToLower(safePath), ".jpeg") {
		ct = "image/jpeg"
	}
	return f, ct, info.Size(), nil
}

func (s *LocalStorageService) ReadFile(storedPath string) (io.ReadCloser, string, int64, error) {
	return s.GetFile(storedPath)
}

func (s *LocalStorageService) DeleteFile(storedPath string) error {
	safePath, err := s.ResolveSafePath(storedPath)
	if err != nil {
		return err
	}
	if err := os.Remove(safePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *LocalStorageService) ResolveSafePath(storedPath string) (string, error) {
	if strings.TrimSpace(storedPath) == "" {
		return "", errors.New("empty file path")
	}
	absBase, err := filepath.Abs(s.BaseDir)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(storedPath)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absTarget, absBase+string(os.PathSeparator)) && absTarget != absBase {
		return "", errors.New("access denied: path traversal detected")
	}
	if _, err := os.Stat(absTarget); os.IsNotExist(err) {
		return "", errors.New("file not found on storage")
	}
	return absTarget, nil
}

// ---------------- S3StorageService ----------------

func (s *S3StorageService) SavePDFFile(fileHeader *multipart.FileHeader, subFolder string) (string, string, int64, error) {
	src, err := validatePDFHeader(fileHeader, s.MaxBytes)
	if err != nil {
		return "", "", 0, err
	}
	defer src.Close()

	safeSubFolder, err := sanitizeSubFolder(subFolder)
	if err != nil {
		return "", "", 0, err
	}

	data, err := io.ReadAll(io.LimitReader(src, s.MaxBytes+1))
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to read uploaded file: %w", err)
	}
	if int64(len(data)) > s.MaxBytes {
		return "", "", 0, fmt.Errorf("file size exceeds maximum limit of %d MB", s.MaxBytes/(1024*1024))
	}

	storedName := fmt.Sprintf("%s.pdf", uuid.New().String())
	objectKey := path.Join(safeSubFolder, storedName)
	contentType := "application/pdf"

	_, err = s.Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &s.Bucket,
		Key:         &objectKey,
		Body:        bytes.NewReader(data),
		ContentType: &contentType,
	})
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to upload PDF to S3: %w", err)
	}

	return storedName, objectKey, int64(len(data)), nil
}

func (s *S3StorageService) SaveImageFile(fileHeader *multipart.FileHeader, subFolder string) (string, string, error) {
	src, ext, contentType, err := validateImageHeader(fileHeader)
	if err != nil {
		return "", "", err
	}
	defer src.Close()

	safeSubFolder, err := sanitizeSubFolder(subFolder)
	if err != nil {
		return "", "", err
	}

	data, err := io.ReadAll(io.LimitReader(src, 5*1024*1024+1))
	if err != nil {
		return "", "", fmt.Errorf("failed to read image: %w", err)
	}

	storedName := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	objectKey := path.Join(safeSubFolder, storedName)

	_, err = s.Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &s.Bucket,
		Key:         &objectKey,
		Body:        bytes.NewReader(data),
		ContentType: &contentType,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to upload image to S3: %w", err)
	}

	return storedName, objectKey, nil
}

func (s *S3StorageService) SaveGeneratedFile(localPath string, subFolder string, contentType string) (string, string, error) {
	safeSubFolder, err := sanitizeSubFolder(subFolder)
	if err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read generated file: %w", err)
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/pdf"
	}

	storedName := filepath.Base(localPath)
	objectKey := path.Join(safeSubFolder, storedName)

	_, err = s.Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &s.Bucket,
		Key:         &objectKey,
		Body:        bytes.NewReader(data),
		ContentType: &contentType,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to upload generated file to S3: %w", err)
	}

	return storedName, objectKey, nil
}

func (s *S3StorageService) GetFile(storedPath string) (io.ReadCloser, string, int64, error) {
	objectKey, err := sanitizeObjectKey(storedPath)
	if err != nil {
		return nil, "", 0, err
	}

	out, err := s.Client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: &s.Bucket,
		Key:    &objectKey,
	})
	if err == nil {
		ct := "application/octet-stream"
		if out.ContentType != nil && *out.ContentType != "" {
			ct = *out.ContentType
		} else if strings.HasSuffix(strings.ToLower(objectKey), ".pdf") {
			ct = "application/pdf"
		}
		var size int64
		if out.ContentLength != nil {
			size = *out.ContentLength
		}
		return out.Body, ct, size, nil
	}

	// Backward compatibility: fallback to local disk if legacy path exists locally
	if s.BaseDir != "" {
		localSvc := &LocalStorageService{BaseDir: s.BaseDir, MaxBytes: s.MaxBytes}
		if reader, ct, size, localErr := localSvc.GetFile(storedPath); localErr == nil {
			return reader, ct, size, nil
		}
	}

	return nil, "", 0, fmt.Errorf("failed to retrieve file from S3: %w", err)
}

func (s *S3StorageService) ReadFile(storedPath string) (io.ReadCloser, string, int64, error) {
	return s.GetFile(storedPath)
}

func (s *S3StorageService) DeleteFile(storedPath string) error {
	objectKey, err := sanitizeObjectKey(storedPath)
	if err != nil {
		return err
	}
	_, err = s.Client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: &s.Bucket,
		Key:    &objectKey,
	})
	if err != nil {
		return fmt.Errorf("failed to delete object from S3: %w", err)
	}
	return nil
}

func (s *S3StorageService) ResolveSafePath(storedPath string) (string, error) {
	return sanitizeObjectKey(storedPath)
}

type pdfHeaderChecker struct{}

var pdfHelper = pdfHeaderChecker{}

func (pdfHeaderChecker) HasPrefixPDF(buf []byte) bool {
	trimmed := strings.TrimSpace(string(buf))
	return strings.HasPrefix(trimmed, "%PDF-")
}
