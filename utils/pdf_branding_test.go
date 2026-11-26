package utils

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

func mergePDFsForTest(inputs []string, outPath string) error {
	return api.MergeCreateFile(context.Background(), inputs, outPath, false, newPDFCPUConfig())
}

func TestBrandResearchPaperPDF(t *testing.T) {
	tmp := t.TempDir()
	inPath, _, err := GenerateSamplePaperPDF(
		tmp,
		"RH-2026-TEST",
		"Automated PDF Branding Verification",
		"Aditya Anerao",
		"ResearchHub Institute",
		"Abstract for testing PDF branding, header, footer, QR code, and watermark.",
		"PDF, Branding, ResearchHub",
	)
	if err != nil {
		t.Fatalf("failed to generate sample input PDF: %v", err)
	}

	outPath := filepath.Join(tmp, "branded.pdf")
	opts := DefaultBrandingOptions(
		"RHPUB-2026-TEST",
		"https://researchhub.buzz/publications/RHPUB-2026-TEST",
		"November 2026",
		"12",
		"4",
		"",
	)

	pages, err := BrandResearchPaperPDF(inPath, outPath, opts, 200)
	if err != nil {
		t.Fatalf("BrandResearchPaperPDF failed: %v", err)
	}
	if pages != 1 {
		t.Fatalf("expected 1 page, got %d", pages)
	}
	info, err := os.Stat(outPath)
	if err != nil || info.Size() < 500 {
		t.Fatalf("expected valid branded output PDF, got err=%v", err)
	}

	// Invalid PDF should be rejected
	badPath := filepath.Join(tmp, "bad.pdf")
	_ = os.WriteFile(badPath, []byte("not a valid pdf"), 0644)
	if _, err := BrandResearchPaperPDF(badPath, filepath.Join(tmp, "bad_out.pdf"), opts, 200); err == nil {
		t.Fatalf("expected invalid PDF to be rejected")
	}
}

func TestBrandRealUploadedStudentPDFWithLogo(t *testing.T) {
	realPDF := filepath.Join("..", "storage", "papers", "022ee844-11c1-4bdf-9f35-ea2dfc9706b1.pdf")
	logoPath := filepath.Join("..", "storage", "logo.png")
	if _, err := os.Stat(realPDF); os.IsNotExist(err) {
		t.Skip("real student PDF not present")
	}
	inInfo, _ := os.Stat(realPDF)

	outPath := filepath.Join(t.TempDir(), "ResearchHub-Sample-Branded.pdf")
	opts := DefaultBrandingOptions(
		"RHPUB-2026-REAL",
		"https://researchhub.buzz/publications/RHPUB-2026-REAL",
		"November 2026",
		"12",
		"4",
		logoPath,
	)

	var mBefore, mAfter runtime.MemStats
	runtime.ReadMemStats(&mBefore)
	start := time.Now()

	pages, err := BrandResearchPaperPDF(realPDF, outPath, opts, 200)
	if err != nil {
		t.Fatalf("BrandResearchPaperPDF failed on real PDF: %v", err)
	}
	dur := time.Since(start)
	runtime.ReadMemStats(&mAfter)
	outInfo, _ := os.Stat(outPath)

	t.Logf("Real PDF branded: pages=%d, inSize=%d bytes, outSize=%d bytes, duration=%v, allocDelta=%d KB",
		pages, inInfo.Size(), outInfo.Size(), dur, (mAfter.TotalAlloc-mBefore.TotalAlloc)/1024)
}

func TestBrand25PagePDFBenchmark(t *testing.T) {
	tmp := t.TempDir()
	singlePath, _, err := GenerateSamplePaperPDF(
		tmp,
		"RH-2026-25P",
		"Herbal Solid Perfume Formulation and Evaluation (25-Page Benchmark)",
		"Aditya Anerao",
		"Aditya Pharmacy College, Beed",
		"Comprehensive 25-page benchmark manuscript with text, metadata, and structured sections.",
		"Herbal, Solid Perfume, Formulation",
	)
	if err != nil {
		t.Fatalf("failed to create sample page: %v", err)
	}

	// Build a 25-page PDF from the sample page
	in25 := filepath.Join(tmp, "input_25p.pdf")
	inputs := make([]string, 25)
	for i := 0; i < 25; i++ {
		inputs[i] = singlePath
	}
	if err := mergePDFsForTest(inputs, in25); err != nil {
		t.Fatalf("failed to build 25-page PDF: %v", err)
	}
	inInfo, _ := os.Stat(in25)

	logoPath := filepath.Join("..", "storage", "logo.png")
	out25 := filepath.Join(tmp, "ResearchHub-25Page-Branded.pdf")
	opts := DefaultBrandingOptions(
		"RHPUB-2026-25P",
		"https://researchhub.buzz/publications/RHPUB-2026-25P",
		"November 2026",
		"12",
		"4",
		logoPath,
	)

	var mBefore, mAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&mBefore)
	start := time.Now()

	pages, err := BrandResearchPaperPDF(in25, out25, opts, 200)
	if err != nil {
		t.Fatalf("BrandResearchPaperPDF 25-page failed: %v", err)
	}
	dur := time.Since(start)
	runtime.ReadMemStats(&mAfter)
	outInfo, _ := os.Stat(out25)

	if pages != 25 {
		t.Fatalf("expected 25 pages, got %d", pages)
	}
	t.Logf("25-Page PDF Benchmark: pages=%d, inSize=%d bytes, outSize=%d bytes, duration=%v, heapInUse=%d KB, sys=%d KB",
		pages, inInfo.Size(), outInfo.Size(), dur, mAfter.HeapInuse/1024, mAfter.Sys/1024)
}
