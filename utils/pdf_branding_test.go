package utils

import (
	"bytes"
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

func TestOverlayHorizontalRotationAndLandscapeSupport(t *testing.T) {
	tmp := t.TempDir()
	opts := DefaultBrandingOptions(
		"RHPUB-2026-ROT0",
		"https://researchhub.buzz/publications/RHPUB-2026-ROT0",
		"November 2026",
		"12",
		"4",
		filepath.Join("..", "storage", "logo.png"),
	)

	// Verify overlay PDF builds cleanly for both A4 Portrait (595x842) and A4 Landscape (842x595)
	for _, dims := range [][2]int{{595, 842}, {842, 595}, {612, 792}} {
		overlay := buildPageOverlayPDF(dims[0], dims[1], 1, 25, opts)
		if !bytes.HasPrefix(overlay, []byte("%PDF-1.4")) {
			t.Fatalf("expected valid PDF overlay for %dx%d", dims[0], dims[1])
		}
		if !bytes.Contains(overlay, []byte("Page 1 of 25")) {
			t.Fatalf("expected Page 1 of 25 in overlay for %dx%d", dims[0], dims[1])
		}
		if !bytes.Contains(overlay, []byte("Hosted on ResearchHub")) {
			t.Fatalf("expected footer text in overlay for %dx%d", dims[0], dims[1])
		}
	}

	// Verify landscape PDF branding preserves landscape dimensions and page count
	landIn := filepath.Join(tmp, "landscape.pdf")
	landBytes := buildSinglePagePDF(842, 595, "BT /F2 14 Tf 100 300 Td (Landscape Research Content) Tj ET")
	_ = os.WriteFile(landIn, landBytes, 0644)
	landOut := filepath.Join(tmp, "landscape_branded.pdf")
	pages, err := BrandResearchPaperPDF(landIn, landOut, opts, 200)
	if err != nil || pages != 1 {
		t.Fatalf("landscape branding failed: pages=%d err=%v", pages, err)
	}
}

func TestGenerateVerificationSamplePDFs(t *testing.T) {
	verDir := filepath.Join("..", "testdata", "verification")
	if err := os.MkdirAll(verDir, 0755); err != nil {
		t.Fatalf("failed to create verification dir: %v", err)
	}
	logoPath := filepath.Join("..", "storage", "logo.png")
	tmp := t.TempDir()

	// 1. Anonymized Synthetic Research Manuscript with edge-to-edge header, footer, and table
	edgeStream := "BT /F2 12 Tf 0.1 0.1 0.1 rg 30 820 Td (Original Top-Edge Manuscript Header - Must Not Overlap ResearchHub Header) Tj ET\n" +
		"BT /F1 11 Tf 0.1 0.1 0.1 rg 30 780 Td (Title: Lattice-Based Cryptography for Edge IoT Devices) Tj ET\n" +
		"BT /F1 10 Tf 0.1 0.1 0.1 rg 30 755 Td (Author: Dr. A. Researcher | Department of Computer Science) Tj ET\n" +
		"0.8 0.8 0.85 RG 1 w 30 400 535 200 re S\n" +
		"BT /F2 10 Tf 0.1 0.1 0.1 rg 45 580 Td (Table 1: Experimental Throughput & Latency Benchmarks) Tj ET\n" +
		"BT /F1 9 Tf 0.7 0.1 0.1 rg 30 22 Td (Original Bottom-Edge Manuscript Footer - Must Not Overlap ResearchHub Footer) Tj ET\n"
	edgeIn := filepath.Join(tmp, "edge_manuscript.pdf")
	_ = os.WriteFile(edgeIn, buildSinglePagePDF(595, 842, edgeStream), 0644)
	outStudent := filepath.Join(verDir, "ResearchHub-Corrected-Student-Paper.pdf")
	opts := DefaultBrandingOptions("RHPUB-2026-STU01", "https://researchhub.buzz/publications/RHPUB-2026-STU01", "November 2026", "12", "4", logoPath)
	if pages, err := BrandResearchPaperPDF(edgeIn, outStudent, opts, 200); err != nil || pages != 1 {
		t.Fatalf("failed generating ResearchHub-Corrected-Student-Paper.pdf: pages=%d err=%v", pages, err)
	}

	// 2. 25-Page Paper
	singlePath, _, err := GenerateSamplePaperPDF(tmp, "RH-2026-25P", "Comprehensive Multi-Page Research Paper", "A. Researcher", "Institute of Technology", "25-page visual verification manuscript.", "Cryptography, Edge IoT")
	if err != nil {
		t.Fatalf("failed generating sample page: %v", err)
	}
	in25 := filepath.Join(tmp, "in25.pdf")
	inputs := make([]string, 25)
	for i := 0; i < 25; i++ {
		inputs[i] = singlePath
	}
	if err := mergePDFsForTest(inputs, in25); err != nil {
		t.Fatalf("failed merging 25 pages: %v", err)
	}
	out25 := filepath.Join(verDir, "ResearchHub-Corrected-25Page-Paper.pdf")
	opts25 := DefaultBrandingOptions("RHPUB-2026-25P", "https://researchhub.buzz/publications/RHPUB-2026-25P", "November 2026", "12", "4", logoPath)
	if pages, err := BrandResearchPaperPDF(in25, out25, opts25, 200); err != nil || pages != 25 {
		t.Fatalf("failed generating ResearchHub-Corrected-25Page-Paper.pdf: pages=%d err=%v", pages, err)
	}

	// 3. Landscape Paper (842 x 595 pt) with edge-to-edge content
	landStream := "BT /F2 12 Tf 0.1 0.1 0.1 rg 30 572 Td (Landscape Top-Edge Header - Must Stay Below ResearchHub Header) Tj ET\n" +
		"BT /F2 14 Tf 0.1 0.1 0.1 rg 120 300 Td (Landscape Research Manuscript Content - Explicit Form XObject Composition) Tj ET\n" +
		"BT /F1 9 Tf 0.7 0.1 0.1 rg 30 18 Td (Landscape Bottom-Edge Footer - Must Stay Above ResearchHub Footer) Tj ET\n"
	landIn := filepath.Join(tmp, "land_in.pdf")
	_ = os.WriteFile(landIn, buildSinglePagePDF(842, 595, landStream), 0644)
	outLand := filepath.Join(verDir, "ResearchHub-Corrected-Landscape.pdf")
	optsLand := DefaultBrandingOptions("RHPUB-2026-LAND", "https://researchhub.buzz/publications/RHPUB-2026-LAND", "November 2026", "12", "4", logoPath)
	if pages, err := BrandResearchPaperPDF(landIn, outLand, optsLand, 200); err != nil || pages != 1 {
		t.Fatalf("failed generating ResearchHub-Corrected-Landscape.pdf: pages=%d err=%v", pages, err)
	}
}
