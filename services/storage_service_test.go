package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type mockS3Client struct {
	objects map[string][]byte
	types   map[string]string
}

func newMockS3Client() *mockS3Client {
	return &mockS3Client{
		objects: make(map[string][]byte),
		types:   make(map[string]string),
	}
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	data, err := io.ReadAll(params.Body)
	if err != nil {
		return nil, err
	}
	m.objects[*params.Key] = data
	if params.ContentType != nil {
		m.types[*params.Key] = *params.ContentType
	}
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	data, ok := m.objects[*params.Key]
	if !ok {
		return nil, errors.New("NoSuchKey")
	}
	ct := m.types[*params.Key]
	size := int64(len(data))
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentType:   &ct,
		ContentLength: &size,
	}, nil
}

func (m *mockS3Client) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	delete(m.objects, *params.Key)
	delete(m.types, *params.Key)
	return &s3.DeleteObjectOutput{}, nil
}

func (m *mockS3Client) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if _, ok := m.objects[*params.Key]; !ok {
		return nil, errors.New("NotFound")
	}
	return &s3.HeadObjectOutput{}, nil
}

func createMultipartFileHeader(t *testing.T, fieldName, fileName string, content []byte) *multipart.FileHeader {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+fieldName+`"; filename="`+fileName+`"`)
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	reqBody := bytes.NewReader(b.Bytes())
	mr := multipart.NewReader(reqBody, w.Boundary())
	form, err := mr.ReadForm(int64(len(content)) + 4096)
	if err != nil {
		t.Fatal(err)
	}
	return form.File[fieldName][0]
}

func TestS3StorageLifecycleAndValidation(t *testing.T) {
	mock := newMockS3Client()
	svc := &S3StorageService{
		Client:   mock,
		Bucket:   "researchimage",
		Region:   "ap-southeast-2",
		BaseDir:  t.TempDir(),
		MaxBytes: 1024 * 1024, // 1 MB for test
	}

	// 1. Valid PDF upload
	pdfBytes := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF\n")
	fh := createMultipartFileHeader(t, "file", "paper.pdf", pdfBytes)
	_, key, size, err := svc.SavePDFFile(fh, "papers")
	if err != nil {
		t.Fatalf("expected valid PDF upload to succeed, got: %v", err)
	}
	if size != int64(len(pdfBytes)) || !strings.HasPrefix(key, "papers/") {
		t.Fatalf("unexpected key=%s size=%d", key, size)
	}

	// 2. S3 Download / ReadFile
	rc, ct, _, err := svc.ReadFile(key)
	if err != nil {
		t.Fatalf("expected ReadFile to succeed, got: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if ct != "application/pdf" || !bytes.Equal(got, pdfBytes) {
		t.Fatalf("downloaded content mismatch")
	}

	// 3. S3 Delete
	if err := svc.DeleteFile(key); err != nil {
		t.Fatalf("expected DeleteFile to succeed, got: %v", err)
	}
	if _, _, _, err := svc.ReadFile(key); err == nil {
		t.Fatalf("expected error reading deleted S3 object")
	}

	// 4. Invalid PDF content
	badPDF := createMultipartFileHeader(t, "file", "fake.pdf", []byte("not a pdf"))
	if _, _, _, err := svc.SavePDFFile(badPDF, "papers"); err == nil {
		t.Fatalf("expected invalid PDF to fail")
	}

	// 5. Oversized PDF
	bigPDF := createMultipartFileHeader(t, "file", "big.pdf", append([]byte("%PDF-1.4\n"), make([]byte, 2*1024*1024)...))
	if _, _, _, err := svc.SavePDFFile(bigPDF, "papers"); err == nil {
		t.Fatalf("expected oversized PDF to fail")
	}

	// 6. Invalid image
	badImg := createMultipartFileHeader(t, "logo", "logo.png", []byte("not an image"))
	if _, _, err := svc.SaveImageFile(badImg, "logos"); err == nil {
		t.Fatalf("expected invalid image to fail")
	}

	// 7. Path traversal / invalid object key
	if _, _, _, err := svc.ReadFile("../../etc/passwd"); err == nil {
		t.Fatalf("expected path traversal key to fail")
	}
}

func TestLocalStorageBackwardCompatibility(t *testing.T) {
	tmp := t.TempDir()
	localSvc := &LocalStorageService{BaseDir: tmp, MaxBytes: 5 * 1024 * 1024, Driver: "local"}

	pdfBytes := []byte("%PDF-1.4\n%%EOF\n")
	fh := createMultipartFileHeader(t, "file", "local.pdf", pdfBytes)
	_, fullPath, _, err := localSvc.SavePDFFile(fh, "papers")
	if err != nil {
		t.Fatalf("local SavePDFFile failed: %v", err)
	}

	// S3StorageService should fall back to local disk for legacy paths
	s3Svc := &S3StorageService{
		Client:   newMockS3Client(),
		Bucket:   "researchimage",
		Region:   "ap-southeast-2",
		BaseDir:  tmp,
		MaxBytes: 5 * 1024 * 1024,
	}
	rc, _, _, err := s3Svc.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("expected S3StorageService fallback to local path %s, got: %v", filepath.Base(fullPath), err)
	}
	_ = rc.Close()
	_ = os.Remove(fullPath)
}
