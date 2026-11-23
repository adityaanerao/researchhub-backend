package utils

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

func escapePDFText(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`(`, `\(`,
		`)`, `\)`,
		"\r", " ",
		"\n", " ",
	)
	return replacer.Replace(s)
}

func wrapText(text string, maxChars int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	current := words[0]
	for _, w := range words[1:] {
		if len(current)+1+len(w) <= maxChars {
			current += " " + w
		} else {
			lines = append(lines, current)
			current = w
		}
	}
	lines = append(lines, current)
	return lines
}

// buildSinglePagePDF wraps a raw PDF content stream into a valid PDF 1.4 document with Helvetica & Helvetica-Bold fonts, and optional PNG logo XObject
func buildSinglePagePDFWithLogo(pageWidth, pageHeight int, contentStream string, logoPath string) []byte {
	var rgbCompressed, alphaCompressed bytes.Buffer
	var imgW, imgH int
	hasLogo := false

	if logoPath != "" {
		if f, err := os.Open(logoPath); err == nil {
			if img, err := png.Decode(f); err == nil {
				bounds := img.Bounds()
				imgW = bounds.Dx()
				imgH = bounds.Dy()
				rgbRaw := make([]byte, 0, imgW*imgH*3)
				alphaRaw := make([]byte, 0, imgW*imgH)
				for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
					for x := bounds.Min.X; x < bounds.Max.X; x++ {
						r, g, b, a := img.At(x, y).RGBA()
						rgbRaw = append(rgbRaw, byte(r>>8), byte(g>>8), byte(b>>8))
						alphaRaw = append(alphaRaw, byte(a>>8))
					}
				}
				zw1 := zlib.NewWriter(&rgbCompressed)
				_, _ = zw1.Write(rgbRaw)
				_ = zw1.Close()

				zw2 := zlib.NewWriter(&alphaCompressed)
				_, _ = zw2.Write(alphaRaw)
				_ = zw2.Close()
				hasLogo = true
			}
			_ = f.Close()
		}
	}

	if !hasLogo {
		return buildSinglePagePDF(pageWidth, pageHeight, contentStream)
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, 9)

	offsets[1] = buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	offsets[2] = buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	offsets[3] = buf.Len()
	buf.WriteString(fmt.Sprintf("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Resources << /Font << /F1 4 0 R /F2 5 0 R >> /XObject << /Logo 7 0 R >> >> /Contents 6 0 R >>\nendobj\n", pageWidth, pageHeight))

	offsets[4] = buf.Len()
	buf.WriteString("4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	offsets[5] = buf.Len()
	buf.WriteString("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>\nendobj\n")

	offsets[6] = buf.Len()
	buf.WriteString(fmt.Sprintf("6 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(contentStream), contentStream))

	offsets[7] = buf.Len()
	buf.WriteString(fmt.Sprintf("7 0 obj\n<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /SMask 8 0 R /Length %d >>\nstream\n", imgW, imgH, rgbCompressed.Len()))
	buf.Write(rgbCompressed.Bytes())
	buf.WriteString("\nendstream\nendobj\n")

	offsets[8] = buf.Len()
	buf.WriteString(fmt.Sprintf("8 0 obj\n<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\nstream\n", imgW, imgH, alphaCompressed.Len()))
	buf.Write(alphaCompressed.Bytes())
	buf.WriteString("\nendstream\nendobj\n")

	xrefPos := buf.Len()
	buf.WriteString("xref\n0 9\n0000000000 65535 f \n")
	for i := 1; i <= 8; i++ {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size 9 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefPos))
	return buf.Bytes()
}

// buildSinglePagePDFWithBgJPEG embeds a JPEG background template full-page and overlays dynamic PDF content
func buildSinglePagePDFWithBgJPEG(pageWidth, pageHeight int, contentStream string, bgPath string) []byte {
	rawJPEG, err := os.ReadFile(bgPath)
	if err != nil {
		return buildSinglePagePDF(pageWidth, pageHeight, contentStream)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(rawJPEG))
	if err != nil {
		return buildSinglePagePDF(pageWidth, pageHeight, contentStream)
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, 9)

	offsets[1] = buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	offsets[2] = buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	offsets[3] = buf.Len()
	buf.WriteString(fmt.Sprintf("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Resources << /Font << /F1 4 0 R /F2 5 0 R /F3 8 0 R >> /XObject << /Bg 7 0 R >> >> /Contents 6 0 R >>\nendobj\n", pageWidth, pageHeight))

	offsets[4] = buf.Len()
	buf.WriteString("4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	offsets[5] = buf.Len()
	buf.WriteString("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>\nendobj\n")

	offsets[6] = buf.Len()
	buf.WriteString(fmt.Sprintf("6 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(contentStream), contentStream))

	offsets[7] = buf.Len()
	buf.WriteString(fmt.Sprintf("7 0 obj\n<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n", cfg.Width, cfg.Height, len(rawJPEG)))
	buf.Write(rawJPEG)
	buf.WriteString("\nendstream\nendobj\n")

	offsets[8] = buf.Len()
	buf.WriteString("8 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Times-Bold >>\nendobj\n")

	xrefPos := buf.Len()
	buf.WriteString("xref\n0 9\n0000000000 65535 f \n")
	for i := 1; i <= 8; i++ {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size 9 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefPos))
	return buf.Bytes()
}

// buildSinglePagePDF wraps a raw PDF content stream into a valid PDF 1.4 document with Helvetica & Helvetica-Bold fonts
func buildSinglePagePDF(pageWidth, pageHeight int, contentStream string) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")

	offsets := make([]int, 7)

	// Object 1: Catalog
	offsets[1] = buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// Object 2: Pages
	offsets[2] = buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	// Object 3: Page
	offsets[3] = buf.Len()
	buf.WriteString(fmt.Sprintf("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Resources << /Font << /F1 4 0 R /F2 5 0 R >> >> /Contents 6 0 R >>\nendobj\n", pageWidth, pageHeight))

	// Object 4: Helvetica
	offsets[4] = buf.Len()
	buf.WriteString("4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	// Object 5: Helvetica-Bold
	offsets[5] = buf.Len()
	buf.WriteString("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>\nendobj\n")

	// Object 6: Content stream
	offsets[6] = buf.Len()
	buf.WriteString(fmt.Sprintf("6 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(contentStream), contentStream))

	xrefPos := buf.Len()
	buf.WriteString("xref\n0 7\n0000000000 65535 f \n")
	for i := 1; i <= 6; i++ {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size 7 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefPos))
	return buf.Bytes()
}

// GenerateQRCodeBase64 returns a base64 PNG data URI for a verification URL
func GenerateQRCodeBase64(content string) (string, error) {
	png, err := qrcode.Encode(content, qrcode.Medium, 220)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

// GenerateCertificatePDF generates an academic landscape PDF certificate matching the official ResearchHub certificate design
func GenerateCertificatePDF(
	storageDir, certCode, brandName, journalName, authorName, institution, paperTitle,
	pubCode, doi, volume, issue, pubDate, signatoryName, signatoryTitle, verifyURL string,
) (string, error) {
	certDir := filepath.Join(storageDir, "certificates")
	_ = os.MkdirAll(certDir, 0755)
	filePath := filepath.Join(certDir, fmt.Sprintf("%s.pdf", certCode))

	// Landscape A4: 842 x 595 pt
	bgTemplatePath := filepath.Join(storageDir, "certificate_template.jpg")
	if _, err := os.Stat(bgTemplatePath); err == nil {
		var overlay bytes.Buffer
		// Draw full-page background template image
		overlay.WriteString("q 842 0 0 595 0 0 cm /Bg Do Q\n")

		// Top-Right Certificate No and Date values
		overlay.WriteString(fmt.Sprintf("BT /F2 9.5 Tf 0.059 0.161 0.259 rg 642 545 Td (%s) Tj ET\n", escapePDFText(certCode)))
		overlay.WriteString(fmt.Sprintf("BT /F2 9.5 Tf 0.059 0.161 0.259 rg 642 529 Td (%s) Tj ET\n", escapePDFText(pubDate)))

		// Dynamic Author Name centered (Times-Bold 26pt)
		authorLines := wrapText(strings.TrimSpace(authorName), 34)
		ay := 374
		if len(authorLines) > 1 {
			ay = 384
		}
		for idx, al := range authorLines {
			if idx >= 2 {
				break
			}
			ax := 421 - int(float64(len(al))*6.5)
			if ax < 140 {
				ax = 140
			}
			overlay.WriteString(fmt.Sprintf("BT /F3 26 Tf 0.059 0.161 0.259 rg %d %d Td (%s) Tj ET\n", ax, ay, escapePDFText(al)))
			ay -= 26
		}

		// Dynamic College / Institution Name centered below Author Name (Times-Bold 13pt)
		if inst := strings.TrimSpace(institution); inst != "" {
			ix := 421 - int(float64(len(inst))*3.3)
			if ix < 110 {
				ix = 110
			}
			overlay.WriteString(fmt.Sprintf("BT /F3 13 Tf 0.12 0.24 0.36 rg %d %d Td (%s) Tj ET\n", ix, 352, escapePDFText(inst)))
		}

		// Dynamic Paper Title centered (Times-Bold 22pt, max size for section)
		titleLines := wrapText("\""+paperTitle+"\"", 42)
		ty := 302
		if len(titleLines) > 1 {
			ty = 314
		}
		for idx, tl := range titleLines {
			if idx >= 2 {
				break
			}
			tx := 421 - int(float64(len(tl))*5.5)
			if tx < 90 {
				tx = 90
			}
			overlay.WriteString(fmt.Sprintf("BT /F3 22 Tf 0.059 0.161 0.259 rg %d %d Td (%s) Tj ET\n", tx, ty, escapePDFText(tl)))
			ty -= 24
		}

		// Dynamic Publication Date in middle column (cleanly below label)
		overlay.WriteString(fmt.Sprintf("BT /F3 10.5 Tf 0.059 0.161 0.259 rg 371 187 Td (%s) Tj ET\n", escapePDFText(pubDate)))

		// Dynamic DOI in right column (use registered DOI or generate one from certCode)
		doiText := strings.TrimSpace(doi)
		if doiText == "" {
			cleanCode := strings.ToLower(strings.ReplaceAll(certCode, "-", "."))
			if cleanCode == "" {
				cleanCode = "rh.2026.0001"
			}
			doiText = "10.5281/" + cleanCode
		}
		overlay.WriteString(fmt.Sprintf("BT /F3 10.5 Tf 0.059 0.161 0.259 rg 558 187 Td (%s) Tj ET\n", escapePDFText(doiText)))

		// Dynamic Vector QR Code over bottom-right QR box
		if qr, qErr := qrcode.New(verifyURL, qrcode.Medium); qErr == nil {
			bitmap := qr.Bitmap()
			n := len(bitmap)
			if n > 0 {
				overlay.WriteString("1 1 1 rg 736 69 68 68 re f\n")
				moduleSize := 64.0 / float64(n)
				qrOriginX := 738.0
				qrOriginY := 71.0
				overlay.WriteString("0.059 0.161 0.259 rg\n")
				for r := 0; r < n; r++ {
					for c := 0; c < n; c++ {
						if bitmap[r][c] {
							x := qrOriginX + float64(c)*moduleSize
							y := qrOriginY + float64(n-1-r)*moduleSize
							overlay.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f re f\n", x, y, moduleSize, moduleSize))
						}
					}
				}
			}
		}

		pdfBytes := buildSinglePagePDFWithBgJPEG(842, 595, overlay.String(), bgTemplatePath)
		if err := os.WriteFile(filePath, pdfBytes, 0644); err != nil {
			return "", err
		}
		return filePath, nil
	}

	// Fallback if template image is not present
	var stream bytes.Buffer

	// White background
	stream.WriteString("1 1 1 rg 0 0 842 595 re f\n")

	// Navy Blue & Gold Double Borders
	stream.WriteString("2.5 w 0.773 0.608 0.153 RG 14 14 814 567 re S\n")
	stream.WriteString("1 w 0.059 0.161 0.259 RG 20 20 802 555 re S\n")

	// Top-Right & Bottom-Left Diagonal Navy & Gold Corner Ribbons
	stream.WriteString("0.059 0.161 0.259 rg 710 595 m 842 595 l 842 480 l f\n")
	stream.WriteString("0.773 0.608 0.153 rg 685 595 m 710 595 l 842 480 l 842 458 l f\n")
	stream.WriteString("0.059 0.161 0.259 rg 0 0 m 132 0 l 0 115 l f\n")
	stream.WriteString("0.773 0.608 0.153 rg 132 0 m 157 0 l 0 137 l 0 115 l f\n")

	// Subtle Flowing Blue Wave Accent Along Bottom
	stream.WriteString("0.925 0.957 0.992 rg 24 24 794 34 re f\n")

	// Top-Left: Embedded ResearchHub Transparent Logo + URL
	logoPath := filepath.Join(storageDir, "logo.png")
	if _, err := os.Stat(logoPath); err == nil {
		stream.WriteString("q 165 0 0 105 36 472 cm /Logo Do Q\n")
	} else {
		stream.WriteString("BT /F2 20 Tf 0.059 0.161 0.259 rg 45 530 Td (Research Hub) Tj ET\n")
		stream.WriteString("BT /F1 9.5 Tf 0.3 0.35 0.42 rg 45 514 Td (Explore . Research . Publish) Tj ET\n")
		stream.WriteString("BT /F1 9 Tf 0.149 0.388 0.922 rg 45 500 Td (https://researchhub.buzz) Tj ET\n")
	}

	// Top-Right: Dynamic Date Only (No Certificate Number)
	stream.WriteString("0.91 0.945 0.988 rg 575 524 175 28 re f\n")
	stream.WriteString(fmt.Sprintf("BT /F2 10.5 Tf 0.059 0.161 0.259 rg 588 534 Td (Date: %s) Tj ET\n", escapePDFText(pubDate)))

	// Main Title with Gold Flanking Lines
	stream.WriteString("1.5 w 0.773 0.608 0.153 RG 75 448 m 195 448 l S\n")
	stream.WriteString("BT /F2 26 Tf 0.059 0.161 0.259 rg 215 440 Td (Research Publication Certificate) Tj ET\n")
	stream.WriteString("1.5 w 0.773 0.608 0.153 RG 645 448 m 767 448 l S\n")

	// Subtitle
	stream.WriteString("BT /F1 13 Tf 0.2 0.25 0.32 rg 360 414 Td (This is to certify that) Tj ET\n")

	// Dynamic Author Name inside Soft Light-Blue Panel
	stream.WriteString("0.886 0.929 0.980 rg 160 368 522 36 re f\n")
	authorLines := wrapText(strings.TrimSpace(authorName), 44)
	ay := 382
	if len(authorLines) > 1 {
		ay = 388
	}
	for idx, al := range authorLines {
		if idx >= 2 {
			break
		}
		ax := 421 - len(al)*6
		if ax < 170 {
			ax = 170
		}
		stream.WriteString(fmt.Sprintf("BT /F2 19 Tf 0.059 0.161 0.259 rg %d %d Td (%s) Tj ET\n", ax, ay, escapePDFText(al)))
		ay -= 18
	}

	stream.WriteString("BT /F1 12.5 Tf 0.2 0.25 0.32 rg 295 348 Td (has successfully published a research paper titled) Tj ET\n")

	// Dynamic Paper Title inside Soft Light-Blue Panel
	stream.WriteString("0.886 0.929 0.980 rg 108 286 626 52 re f\n")
	titleLines := wrapText("\""+paperTitle+"\"", 64)
	yPos := 318
	for idx, line := range titleLines {
		if idx >= 2 {
			break
		}
		lineX := 421 - len(line)*4
		if lineX < 120 {
			lineX = 120
		}
		stream.WriteString(fmt.Sprintf("BT /F2 13.5 Tf 0.059 0.161 0.259 rg %d %d Td (%s) Tj ET\n", lineX, yPos, escapePDFText(line)))
		yPos -= 18
	}

	// Direct Publication & Hosting Description (No peer-review claim)
	stream.WriteString("BT /F1 11.5 Tf 0.2 0.25 0.32 rg 180 264 Td (on ResearchHub, an online research publication and hosting platform.) Tj ET\n")

	// 3-Column Publication Info Row (Journal Name | Publication Date | DOI if registered)
	stream.WriteString("1 w 0.2 0.25 0.32 RG 330 206 m 330 236 l S\n")
	stream.WriteString("1 w 0.2 0.25 0.32 RG 515 206 m 515 236 l S\n")

	jName := strings.TrimSpace(journalName)
	if jName == "" {
		jName = "Research Hub"
	}
	stream.WriteString("BT /F1 9.5 Tf 0.4 0.45 0.52 rg 180 226 Td (Journal Name) Tj ET\n")
	stream.WriteString(fmt.Sprintf("BT /F2 10 Tf 0.059 0.161 0.259 rg 180 210 Td (%s) Tj ET\n", escapePDFText(truncateStr(jName, 24))))

	stream.WriteString("BT /F1 9.5 Tf 0.4 0.45 0.52 rg 355 226 Td (Publication Date) Tj ET\n")
	stream.WriteString(fmt.Sprintf("BT /F2 10 Tf 0.059 0.161 0.259 rg 355 210 Td (%s) Tj ET\n", escapePDFText(pubDate)))

	stream.WriteString("BT /F1 9.5 Tf 0.4 0.45 0.52 rg 540 226 Td (DOI) Tj ET\n")
	if strings.TrimSpace(doi) != "" {
		stream.WriteString(fmt.Sprintf("BT /F2 10 Tf 0.059 0.161 0.259 rg 540 210 Td (%s) Tj ET\n", escapePDFText(doi)))
	} else {
		stream.WriteString("BT /F1 9.5 Tf 0.45 0.5 0.56 rg 540 210 Td (Not Assigned) Tj ET\n")
	}

	// Blue Laurel Wreaths on Both Sides
	stream.WriteString("0.82 0.89 0.97 rg\n")
	for i := 0; i < 6; i++ {
		ly := 265 + i*18
		stream.WriteString(fmt.Sprintf("64 %d 10 5 re f\n", ly))
		stream.WriteString(fmt.Sprintf("768 %d 10 5 re f\n", ly))
	}

	// Central Gold Publication Seal at Bottom Center (No Signatures)
	stream.WriteString("0.059 0.161 0.259 rg 392 64 m 415 100 l 428 100 l 405 64 l f\n")
	stream.WriteString("0.059 0.161 0.259 rg 450 64 m 427 100 l 414 100 l 437 64 l f\n")
	stream.WriteString("0.855 0.647 0.125 rg 392 98 58 58 re f\n")
	stream.WriteString("0.059 0.161 0.259 RG 1.5 w 397 103 48 48 re S\n")
	stream.WriteString("BT /F2 13 Tf 0.059 0.161 0.259 rg 410 122 Td (RH) Tj ET\n")

	// Bottom-Right: Unique Dynamic Vector QR Code
	qr, err := qrcode.New(verifyURL, qrcode.Medium)
	if err == nil {
		bitmap := qr.Bitmap()
		n := len(bitmap)
		if n > 0 {
			moduleSize := 2.0
			qrOriginX := 732.0
			qrOriginY := 72.0
			stream.WriteString("0.059 0.161 0.259 rg\n")
			for r := 0; r < n; r++ {
				for c := 0; c < n; c++ {
					if bitmap[r][c] {
						x := qrOriginX + float64(c)*moduleSize
						y := qrOriginY + float64(n-1-r)*moduleSize
						stream.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f re f\n", x, y, moduleSize, moduleSize))
					}
				}
			}
		}
	}
	stream.WriteString("BT /F2 8 Tf 0.059 0.161 0.259 rg 732 58 Td (Verify Certificate) Tj ET\n")

	// Exact Required Footer Disclaimer
	stream.WriteString("BT /F1 6.8 Tf 0.35 0.4 0.48 rg 40 38 Td (This certificate confirms the publication and hosting of the specified research paper on ResearchHub. It does not represent peer-reviewed acceptance,) Tj ET\n")
	stream.WriteString("BT /F1 6.8 Tf 0.35 0.4 0.48 rg 40 28 Td (university affiliation, government authorization, academic accreditation or indexing recognition.) Tj ET\n")

	pdfBytes := buildSinglePagePDFWithLogo(842, 595, stream.String(), logoPath)
	if err := os.WriteFile(filePath, pdfBytes, 0644); err != nil {
		return "", err
	}
	return filePath, nil
}

// GenerateSamplePaperPDF creates a valid academic manuscript PDF file on disk
func GenerateSamplePaperPDF(storageDir, submissionCode, title, authors, institution, abstract, keywords string) (string, int64, error) {
	papersDir := filepath.Join(storageDir, "papers")
	_ = os.MkdirAll(papersDir, 0755)
	filePath := filepath.Join(papersDir, fmt.Sprintf("%s.pdf", submissionCode))

	var stream bytes.Buffer
	// Header banner
	stream.WriteString("0.118 0.161 0.231 rg 40 770 515 45 re f\n")
	stream.WriteString(fmt.Sprintf("BT /F2 12 Tf 1 1 1 rg 55 792 Td (RESEARCHHUB MANUSCRIPT REPOSITORY  |  ID: %s) Tj ET\n", escapePDFText(submissionCode)))
	stream.WriteString("BT /F1 9 Tf 0.85 0.88 0.95 rg 55 778 Td (Peer-Reviewed Scholarly Publication System) Tj ET\n")

	// Title
	y := 735
	for _, line := range wrapText(title, 70) {
		stream.WriteString(fmt.Sprintf("BT /F2 15 Tf 0.118 0.161 0.231 rg 45 %d Td (%s) Tj ET\n", y, escapePDFText(line)))
		y -= 20
	}

	// Authors & Institution
	y -= 8
	stream.WriteString(fmt.Sprintf("BT /F2 11 Tf 0.427 0.157 0.851 rg 45 %d Td (Authors: %s) Tj ET\n", y, escapePDFText(authors)))
	y -= 16
	stream.WriteString(fmt.Sprintf("BT /F1 10 Tf 0.3 0.35 0.4 rg 45 %d Td (Affiliation: %s) Tj ET\n", y, escapePDFText(institution)))

	// Divider
	y -= 14
	stream.WriteString(fmt.Sprintf("0.8 0.82 0.86 RG 1 w 45 %d m 550 %d l S\n", y, y))

	// Abstract
	y -= 24
	stream.WriteString(fmt.Sprintf("BT /F2 12 Tf 0.118 0.161 0.231 rg 45 %d Td (Abstract) Tj ET\n", y))
	y -= 18
	for _, line := range wrapText(abstract, 85) {
		stream.WriteString(fmt.Sprintf("BT /F1 10 Tf 0.15 0.18 0.22 rg 45 %d Td (%s) Tj ET\n", y, escapePDFText(line)))
		y -= 15
	}

	// Keywords
	y -= 12
	stream.WriteString(fmt.Sprintf("BT /F2 10 Tf 0.118 0.161 0.231 rg 45 %d Td (Keywords: ) Tj ET\n", y))
	stream.WriteString(fmt.Sprintf("BT /F1 10 Tf 0.25 0.3 0.35 rg 105 %d Td (%s) Tj ET\n", y, escapePDFText(keywords)))

	// Body Section Summary
	y -= 30
	stream.WriteString(fmt.Sprintf("BT /F2 12 Tf 0.118 0.161 0.231 rg 45 %d Td (1. Introduction & Research Context) Tj ET\n", y))
	y -= 18
	intro := "This manuscript presents original empirical and theoretical research submitted to ResearchHub. All experimental evaluations, mathematical formulations, and architectural benchmarks have been validated in accordance with institutional and editorial guidelines."
	for _, line := range wrapText(intro, 85) {
		stream.WriteString(fmt.Sprintf("BT /F1 10 Tf 0.15 0.18 0.22 rg 45 %d Td (%s) Tj ET\n", y, escapePDFText(line)))
		y -= 15
	}

	pdfBytes := buildSinglePagePDF(595, 842, stream.String())
	if err := os.WriteFile(filePath, pdfBytes, 0644); err != nil {
		return "", 0, err
	}
	return filePath, int64(len(pdfBytes)), nil
}

// GenerateReportPDF creates a downloadable tabular PDF report from real database rows
func GenerateReportPDF(reportTitle, subtitle string, headers []string, rows [][]string) []byte {
	var stream bytes.Buffer
	stream.WriteString("0.118 0.161 0.231 rg 30 525 782 45 re f\n")
	stream.WriteString(fmt.Sprintf("BT /F2 15 Tf 1 1 1 rg 45 548 Td (RESEARCHHUB ANALYTICS  -  %s) Tj ET\n", escapePDFText(strings.ToUpper(reportTitle))))
	stream.WriteString(fmt.Sprintf("BT /F1 9 Tf 0.85 0.88 0.95 rg 45 533 Td (%s) Tj ET\n", escapePDFText(subtitle)))

	y := 495
	// Table Header
	stream.WriteString("0.92 0.94 0.97 rg 30 488 782 22 re f\n")
	colWidth := 760 / maxInt(len(headers), 1)
	for i, h := range headers {
		x := 38 + i*colWidth
		stream.WriteString(fmt.Sprintf("BT /F2 9 Tf 0.118 0.161 0.231 rg %d %d Td (%s) Tj ET\n", x, y, escapePDFText(truncateStr(h, 22))))
	}
	y -= 22

	maxRows := 20
	for idx, row := range rows {
		if idx >= maxRows {
			break
		}
		if idx%2 == 1 {
			stream.WriteString(fmt.Sprintf("0.97 0.98 0.99 rg 30 %d 782 18 re f\n", y-4))
		}
		for i, cell := range row {
			if i >= len(headers) {
				break
			}
			x := 38 + i*colWidth
			stream.WriteString(fmt.Sprintf("BT /F1 8 Tf 0.2 0.23 0.28 rg %d %d Td (%s) Tj ET\n", x, y, escapePDFText(truncateStr(cell, 24))))
		}
		y -= 19
	}

	stream.WriteString(fmt.Sprintf("BT /F1 8 Tf 0.4 0.45 0.5 rg 35 25 Td (Total Records Included: %d   |   Generated by ResearchHub Reporting Engine) Tj ET\n", len(rows)))
	return buildSinglePagePDF(842, 595, stream.String())
}

// GeneratePaymentBillPDF creates an official ResearchHub payment receipt PDF for the ₹1000 submission fee
func GeneratePaymentBillPDF(storageDir, receiptNo, studentName, studentEmail, institution, paperTitle, rzpPaymentID, rzpOrderID, paidDate string, amountINR int) (string, error) {
	billsDir := filepath.Join(storageDir, "bills")
	_ = os.MkdirAll(billsDir, 0755)
	filePath := filepath.Join(billsDir, fmt.Sprintf("%s.pdf", receiptNo))

	var stream bytes.Buffer
	// Header Banner
	stream.WriteString("0.059 0.161 0.259 rg 36 740 523 68 re f\n")
	stream.WriteString("BT /F2 20 Tf 1 1 1 rg 52 778 Td (RESEARCH HUB) Tj ET\n")
	stream.WriteString("BT /F1 10 Tf 0.85 0.9 0.96 rg 52 758 Td (Official Payment Receipt & Submission Fee Bill) Tj ET\n")
	stream.WriteString("BT /F2 12 Tf 0.52 0.88 0.58 rg 450 772 Td (STATUS: PAID) Tj ET\n")

	// Receipt Details
	stream.WriteString(fmt.Sprintf("BT /F2 11 Tf 0.059 0.161 0.259 rg 42 708 Td (Receipt No: %s) Tj ET\n", escapePDFText(receiptNo)))
	stream.WriteString(fmt.Sprintf("BT /F1 10 Tf 0.25 0.3 0.36 rg 360 708 Td (Payment Date: %s) Tj ET\n", escapePDFText(paidDate)))
	stream.WriteString(fmt.Sprintf("BT /F1 10 Tf 0.25 0.3 0.36 rg 42 690 Td (Razorpay Payment ID: %s) Tj ET\n", escapePDFText(rzpPaymentID)))
	stream.WriteString(fmt.Sprintf("BT /F1 10 Tf 0.25 0.3 0.36 rg 42 674 Td (Razorpay Order ID: %s) Tj ET\n", escapePDFText(rzpOrderID)))

	stream.WriteString("0.82 0.85 0.90 RG 1 w 42 658 m 553 658 l S\n")

	// Billed To
	stream.WriteString("BT /F2 11 Tf 0.059 0.161 0.259 rg 42 636 Td (Billed To:) Tj ET\n")
	stream.WriteString(fmt.Sprintf("BT /F1 10.5 Tf 0.15 0.18 0.22 rg 42 618 Td (Student / Author: %s) Tj ET\n", escapePDFText(studentName)))
	stream.WriteString(fmt.Sprintf("BT /F1 10.5 Tf 0.15 0.18 0.22 rg 42 602 Td (Email: %s) Tj ET\n", escapePDFText(studentEmail)))
	stream.WriteString(fmt.Sprintf("BT /F1 10.5 Tf 0.15 0.18 0.22 rg 42 586 Td (Institution: %s) Tj ET\n", escapePDFText(institution)))

	// Line Item Table
	stream.WriteString("0.93 0.95 0.98 rg 42 532 511 26 re f\n")
	stream.WriteString("BT /F2 10 Tf 0.059 0.161 0.259 rg 52 541 Td (Description) Tj ET\n")
	stream.WriteString("BT /F2 10 Tf 0.059 0.161 0.259 rg 450 541 Td (Amount (INR)) Tj ET\n")

	stream.WriteString("BT /F1 10 Tf 0.15 0.18 0.22 rg 52 508 Td (Research Paper Submission & Peer-Review Processing Fee) Tj ET\n")
	stream.WriteString(fmt.Sprintf("BT /F2 10.5 Tf 0.059 0.161 0.259 rg 450 508 Td (Rs. %d.00) Tj ET\n", amountINR))

	if paperTitle != "" {
		lines := wrapText("Paper: "+paperTitle, 68)
		py := 488
		for idx, l := range lines {
			if idx >= 2 {
				break
			}
			stream.WriteString(fmt.Sprintf("BT /F1 9 Tf 0.4 0.45 0.52 rg 52 %d Td (%s) Tj ET\n", py, escapePDFText(l)))
			py -= 14
		}
	}

	stream.WriteString("0.82 0.85 0.90 RG 1 w 42 444 m 553 444 l S\n")
	stream.WriteString(fmt.Sprintf("BT /F2 13 Tf 0.059 0.161 0.259 rg 340 420 Td (Total Paid:   Rs. %d.00 INR) Tj ET\n", amountINR))

	stream.WriteString("BT /F1 8.5 Tf 0.4 0.45 0.5 rg 42 60 Td (ResearchHub Academic Press  |  supportinfo.researchhub@gmail.com  |  Computer-generated official receipt) Tj ET\n")

	pdfBytes := buildSinglePagePDF(595, 842, stream.String())
	if err := os.WriteFile(filePath, pdfBytes, 0644); err != nil {
		return "", err
	}
	return filePath, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
