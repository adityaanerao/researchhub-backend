package utils

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	qrcode "github.com/skip2/go-qrcode"
)

// BrandingOptions configures ResearchHub PDF branding for every page.
type BrandingOptions struct {
	BrandName        string  `json:"brand_name"`
	JournalTitle     string  `json:"journal_title"`
	Subtitle         string  `json:"subtitle"`
	Website          string  `json:"website"`
	Volume           string  `json:"volume"`
	Issue            string  `json:"issue"`
	PublicationDate  string  `json:"publication_date"`
	PublicationCode  string  `json:"publication_code"`
	VerificationURL  string  `json:"verification_url"`
	Disclaimer       string  `json:"disclaimer"`
	LogoPath         string  `json:"logo_path"`
	HeaderEnabled    bool    `json:"header_enabled"`
	FooterEnabled    bool    `json:"footer_enabled"`
	QREnabled        bool    `json:"qr_enabled"`
	PageNumbers      bool    `json:"page_numbers_enabled"`
	WatermarkEnabled bool    `json:"watermark_enabled"`
	WatermarkText    string  `json:"watermark_text"`
	WatermarkOpacity float64 `json:"watermark_opacity"`
	WatermarkRot     float64 `json:"watermark_rotation"`
	WatermarkSize    int     `json:"watermark_font_size"`
}

// DefaultBrandingOptions returns production defaults for ResearchHub PDF branding.
func DefaultBrandingOptions(pubCode, verifyURL, pubDate, vol, iss, logoPath string) BrandingOptions {
	return BrandingOptions{
		BrandName:        "ResearchHub",
		JournalTitle:     "International Journal of Academic Research & Innovation",
		Subtitle:         "Open-Access Academic Research Repository",
		Website:          "https://researchhub.buzz",
		Volume:           vol,
		Issue:            iss,
		PublicationDate:  pubDate,
		PublicationCode:  pubCode,
		VerificationURL:  verifyURL,
		Disclaimer:       "Author-submitted research hosted on ResearchHub. Not peer-reviewed.",
		LogoPath:         logoPath,
		HeaderEnabled:    true,
		FooterEnabled:    true,
		QREnabled:        true,
		PageNumbers:      true,
		WatermarkEnabled: true,
		WatermarkText:    "RESEARCHHUB",
		WatermarkOpacity: 0.05,
		WatermarkRot:     45,
		WatermarkSize:    54,
	}
}

func newPDFCPUConfig() *model.Configuration {
	model.ConfigPath = "disable"
	return model.NewDefaultConfiguration()
}

// InspectAndValidatePDF validates PDF structure, rejects encrypted/password-protected PDFs,
// and returns the page count.
func InspectAndValidatePDF(pdfPath string, maxPages int) (int, error) {
	ctx := context.Background()
	conf := newPDFCPUConfig()
	if err := api.ValidateFile(ctx, pdfPath, conf, nil); err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "encrypt") || strings.Contains(msg, "password") {
			return 0, fmt.Errorf("password-protected or encrypted PDFs are not supported")
		}
		return 0, fmt.Errorf("invalid or corrupted PDF: %w", err)
	}
	pageCount, err := api.PageCountFile(ctx, pdfPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read PDF page count: %w", err)
	}
	if pageCount <= 0 {
		return 0, fmt.Errorf("PDF contains no pages")
	}
	if maxPages > 0 && pageCount > maxPages {
		return 0, fmt.Errorf("PDF page count (%d) exceeds maximum limit (%d)", pageCount, maxPages)
	}
	return pageCount, nil
}

// BrandResearchPaperPDF applies ResearchHub header, footer, QR code, page numbering,
// and diagonal watermark to every page of inputPDFPath without modifying original academic content.
func BrandResearchPaperPDF(inputPDFPath, outputPDFPath string, opts BrandingOptions, maxPages int) (int, error) {
	pageCount, err := InspectAndValidatePDF(inputPDFPath, maxPages)
	if err != nil {
		return 0, err
	}

	tmpDir, err := os.MkdirTemp("", "rh_brand_*")
	if err != nil {
		return 0, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 1. Scale original content slightly (0.86) and center so header (top) and footer (bottom)
	// never overlap original text, tables, or diagrams.
	ctx := context.Background()
	scaledPath := filepath.Join(tmpDir, "scaled.pdf")
	nupConf := newPDFCPUConfig()
	nup, nErr := api.PDFNUpConfig(1, "scale:0.86, border:off", nupConf)
	if nErr != nil || api.NUpFile(ctx, []string{inputPDFPath}, scaledPath, nil, nup, nupConf) != nil {
		// Fallback: copy input directly if NUp scaling isn't needed/supported for a specific PDF
		raw, rErr := os.ReadFile(inputPDFPath)
		if rErr != nil {
			return 0, rErr
		}
		if wErr := os.WriteFile(scaledPath, raw, 0644); wErr != nil {
			return 0, wErr
		}
	}

	// 2. Build single-page overlay PDF for each page (with Page X of Y) or stamp per page
	dims, err := api.PageDimsFile(ctx, scaledPath)
	if err != nil || len(dims) == 0 {
		return 0, fmt.Errorf("failed to read page dimensions: %w", err)
	}

	currentIn := scaledPath

	// Apply diagonal watermark across all pages if enabled
	if opts.WatermarkEnabled && strings.TrimSpace(opts.WatermarkText) != "" {
		wmOut := filepath.Join(tmpDir, "watermarked.pdf")
		opacity := opts.WatermarkOpacity
		if opacity <= 0 || opacity > 0.5 {
			opacity = 0.05
		}
		rot := opts.WatermarkRot
		if rot == 0 {
			rot = 45
		}
		fontSize := opts.WatermarkSize
		if fontSize <= 0 {
			fontSize = 54
		}
		wmDesc := fmt.Sprintf("font:Helvetica-Bold, points:%d, color:#1e3a8a, op:%.2f, rot:%.0f, pos:c, scale:1 abs",
			fontSize, opacity, rot)
		wm, wErr := api.TextWatermark(ctx, opts.WatermarkText, wmDesc, false, false, types.POINTS, newPDFCPUConfig())
		if wErr == nil {
			if sErr := api.AddWatermarksFile(ctx, currentIn, wmOut, nil, wm, newPDFCPUConfig()); sErr == nil {
				currentIn = wmOut
			}
		}
	}

	// 3. Stamp header + footer + QR + Page X of Y on each page
	for p := 1; p <= pageCount; p++ {
		d := dims[0]
		if p-1 < len(dims) {
			d = dims[p-1]
		}
		pw := int(d.Width)
		ph := int(d.Height)
		if pw < 200 {
			pw = 595
		}
		if ph < 200 {
			ph = 842
		}

		stampPDFPath := filepath.Join(tmpDir, fmt.Sprintf("stamp_%d.pdf", p))
		stampBytes := buildPageOverlayPDF(pw, ph, p, pageCount, opts)
		if wErr := os.WriteFile(stampPDFPath, stampBytes, 0644); wErr != nil {
			return 0, wErr
		}

		nextOut := filepath.Join(tmpDir, fmt.Sprintf("page_%d.pdf", p))
		wm, wErr := api.PDFWatermark(ctx, stampPDFPath, "scale:1 abs, op:1.0", true, false, types.POINTS, newPDFCPUConfig())
		if wErr != nil {
			return 0, fmt.Errorf("failed to prepare page %d overlay: %w", p, wErr)
		}
		pageSel := []string{fmt.Sprintf("%d", p)}
		if sErr := api.AddWatermarksFile(ctx, currentIn, nextOut, pageSel, wm, newPDFCPUConfig()); sErr != nil {
			return 0, fmt.Errorf("failed to stamp page %d: %w", p, sErr)
		}
		currentIn = nextOut
	}

	// 4. Write final output and validate
	finalBytes, err := os.ReadFile(currentIn)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(outputPDFPath), 0755); err != nil {
		return 0, err
	}
	if err := os.WriteFile(outputPDFPath, finalBytes, 0644); err != nil {
		return 0, err
	}
	if _, err := InspectAndValidatePDF(outputPDFPath, maxPages); err != nil {
		return 0, fmt.Errorf("generated PDF failed validation: %w", err)
	}

	return pageCount, nil
}

func buildPageOverlayPDF(pw, ph, pageNum, totalPages int, opts BrandingOptions) []byte {
	var s bytes.Buffer

	hasLogo := false
	logoW := 28.0
	logoH := 28.0
	if opts.LogoPath != "" {
		if f, err := os.Open(opts.LogoPath); err == nil {
			if cfg, err := png.DecodeConfig(f); err == nil && cfg.Height > 0 {
				logoW = logoH * (float64(cfg.Width) / float64(cfg.Height))
				hasLogo = true
			}
			_ = f.Close()
		}
	}

	// Header (top 68pt)
	if opts.HeaderEnabled {
		topY := ph - 18
		textX := 36
		if hasLogo {
			s.WriteString(fmt.Sprintf("q %.2f 0 0 %.2f 36 %d cm /Logo Do Q\n", logoW, logoH, ph-38))
			textX = 36 + int(logoW) + 8
		}
		brand := opts.BrandName
		if brand == "" {
			brand = "ResearchHub"
		}
		s.WriteString(fmt.Sprintf("BT /F2 11 Tf 0.043 0.122 0.227 rg %d %d Td (%s) Tj ET\n", textX, topY, escapePDFText(brand)))
		if opts.JournalTitle != "" {
			s.WriteString(fmt.Sprintf("BT /F1 8 Tf 0.28 0.34 0.42 rg %d %d Td (%s) Tj ET\n", textX, topY-10, escapePDFText(opts.JournalTitle)))
		}
		if opts.Subtitle != "" {
			s.WriteString(fmt.Sprintf("BT /F1 7 Tf 0.40 0.46 0.54 rg %d %d Td (%s) Tj ET\n", textX, topY-19, escapePDFText(opts.Subtitle)))
		}
		meta := ""
		if opts.Volume != "" || opts.Issue != "" {
			meta = fmt.Sprintf("Vol. %s, Issue %s", opts.Volume, opts.Issue)
		}
		if opts.PublicationDate != "" {
			if meta != "" {
				meta += " | "
			}
			meta += opts.PublicationDate
		}
		if meta != "" {
			mx := pw - 36 - len(meta)*4
			if mx < 260 {
				mx = 260
			}
			s.WriteString(fmt.Sprintf("BT /F1 8 Tf 0.28 0.34 0.42 rg %d %d Td (%s) Tj ET\n", mx, topY, escapePDFText(meta)))
		}
		// Navy & Gold separator lines
		s.WriteString(fmt.Sprintf("0.043 0.122 0.227 RG 1 w 36 %d m %d %d l S\n", topY-25, pw-36, topY-25))
		s.WriteString(fmt.Sprintf("0.78 0.60 0.18 RG 0.8 w 36 %d m %d %d l S\n", topY-27, pw-36, topY-27))
	}

	// Footer (bottom 48pt)
	if opts.FooterEnabled {
		s.WriteString(fmt.Sprintf("0.82 0.85 0.90 RG 0.6 w 36 44 m %d 44 l S\n", pw-36))
		s.WriteString("BT /F2 7.5 Tf 0.043 0.122 0.227 rg 36 33 Td (Hosted on ResearchHub | https://researchhub.buzz) Tj ET\n")
		if opts.Disclaimer != "" {
			s.WriteString(fmt.Sprintf("BT /F1 6.5 Tf 0.40 0.45 0.52 rg 36 22 Td (%s) Tj ET\n", escapePDFText(opts.Disclaimer)))
		}
		if opts.PublicationCode != "" {
			pubInfo := fmt.Sprintf("ID: %s", opts.PublicationCode)
			cx := pw/2 - len(pubInfo)*2
			s.WriteString(fmt.Sprintf("BT /F2 7.5 Tf 0.043 0.122 0.227 rg %d 33 Td (%s) Tj ET\n", cx, escapePDFText(pubInfo)))
		}
		if opts.PageNumbers {
			pgText := fmt.Sprintf("Page %d of %d", pageNum, totalPages)
			px := pw - 82 - len(pgText)*4
			s.WriteString(fmt.Sprintf("BT /F2 7.5 Tf 0.043 0.122 0.227 rg %d 33 Td (%s) Tj ET\n", px, escapePDFText(pgText)))
		}
		if opts.QREnabled && opts.VerificationURL != "" {
			if qr, qErr := qrcode.New(opts.VerificationURL, qrcode.Medium); qErr == nil {
				bm := qr.Bitmap()
				n := len(bm)
				if n > 0 {
					qrSize := 30.0
					ms := qrSize / float64(n)
					ox := float64(pw - 68)
					oy := 10.0
					s.WriteString("0.043 0.122 0.227 rg\n")
					for r := 0; r < n; r++ {
						for c := 0; c < n; c++ {
							if bm[r][c] {
								s.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f re f\n",
									ox+float64(c)*ms, oy+float64(n-1-r)*ms, ms, ms))
							}
						}
					}
				}
			}
		}
	}

	if hasLogo {
		return buildSinglePagePDFWithLogo(pw, ph, s.String(), opts.LogoPath)
	}
	return buildSinglePagePDF(pw, ph, s.String())
}
