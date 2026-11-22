package utils

import (
	"bytes"
	"os"
	"testing"
)

func TestGenerateCertificatePDFVariants(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Short title, no DOI
	p1, err := GenerateCertificatePDF(
		tmpDir, "RH-2026-0001-A1", "ResearchHub", "Research Hub Journal",
		"Aditya Vijay Anerao", "Siddhant College of Engineering, Pune",
		"AI in Healthcare", "RHPUB-2026-001", "", "12", "4", "November 22, 2026",
		"Prof. Rahul Deshmukh", "Academic Director, Research Hub",
		"https://researchhub.buzz/verify-certificate/RH-2026-0001-A1",
	)
	if err != nil {
		t.Fatalf("Short title certificate failed: %v", err)
	}
	b1, _ := os.ReadFile(p1)
	if !bytes.HasPrefix(b1, []byte("%PDF-1.4")) {
		t.Fatalf("Expected valid PDF header")
	}

	// 2. Long title, long author name, registered DOI
	p2, err := GenerateCertificatePDF(
		tmpDir, "RH-2026-0002-A2", "ResearchHub", "Research Hub Journal",
		"Dr. Sukale Pramit Shitalkumar Deshmukh-Patil", "Aditya Pharmacy College, Beed, Maharashtra",
		"Development and Estimation of Carvedilol Tablet in Oral Dosage Form Using Validated RP-HPLC Analytical Method",
		"RHPUB-2026-002", "10.5281/rh.2026.002", "12", "4", "November 22, 2026",
		"Prof. Rahul Deshmukh", "Academic Director, Research Hub",
		"https://researchhub.buzz/verify-certificate/RH-2026-0002-A2",
	)
	if err != nil {
		t.Fatalf("Long title certificate failed: %v", err)
	}
	b2, _ := os.ReadFile(p2)
	if !bytes.HasPrefix(b2, []byte("%PDF-1.4")) {
		t.Fatalf("Expected valid PDF header for long title")
	}
}
