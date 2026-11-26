package utils

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"math"
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

// BrandResearchPaperPDF applies explicit per-page PDF composition (Form XObject placement inside
// reserved manuscript bounds) plus ResearchHub header, footer, QR code, page numbering, and
// diagonal watermark without modifying original academic content.
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

	ctx := context.Background()
	origDims, err := api.PageDimsFile(ctx, inputPDFPath)
	if err != nil || len(origDims) == 0 {
		return 0, fmt.Errorf("failed to read page dimensions: %w", err)
	}

	splitDir := filepath.Join(tmpDir, "split")
	if err := os.MkdirAll(splitDir, 0755); err != nil {
		return 0, err
	}
	if err := api.SplitFile(ctx, inputPDFPath, splitDir, 1, newPDFCPUConfig()); err != nil {
		return 0, fmt.Errorf("failed to split input PDF pages: %w", err)
	}

	// Find split page files sorted by page number
	baseName := strings.TrimSuffix(filepath.Base(inputPDFPath), filepath.Ext(inputPDFPath))
	brandedPages := make([]string, 0, pageCount)

	const (
		headerHeight = 72.0
		footerHeight = 65.0
		leftMargin   = 20.0
		rightMargin  = 20.0
		safetyGap    = 10.0
	)

	for p := 1; p <= pageCount; p++ {
		d := origDims[0]
		if p-1 < len(origDims) {
			d = origDims[p-1]
		}
		srcW := d.Width
		srcH := d.Height
		if srcW < 100 || srcH < 100 {
			return 0, fmt.Errorf("page %d has invalid dimensions (%.1fx%.1f)", p, srcW, srcH)
		}

		pageW := srcW
		pageH := srcH
		if pageW < 400 {
			pageW = 595
		}
		if pageH < 400 {
			pageH = 842
		}

		availW := pageW - leftMargin - rightMargin
		availH := pageH - headerHeight - footerHeight - 2*safetyGap
		if availW <= 0 || availH <= 0 {
			return 0, fmt.Errorf("page %d dimensions too small for header/footer layout", p)
		}

		scale := math.Min(math.Min(availW/srcW, availH/srcH), 1.0)
		scaledW := srcW * scale
		scaledH := srcH * scale
		offsetX := leftMargin + (availW-scaledW)/2.0
		offsetY := footerHeight + safetyGap + (availH-scaledH)/2.0

		// Locate split single-page file
		splitPagePath := filepath.Join(splitDir, fmt.Sprintf("%s_%d.pdf", baseName, p))
		if _, sErr := os.Stat(splitPagePath); sErr != nil {
			// Fallback glob if pdfcpu zero-padded the page suffix (e.g. _01.pdf)
			matches, _ := filepath.Glob(filepath.Join(splitDir, fmt.Sprintf("*_%d.pdf", p)))
			if len(matches) == 0 {
				matches, _ = filepath.Glob(filepath.Join(splitDir, fmt.Sprintf("*_%0*d.pdf", len(fmt.Sprintf("%d", pageCount)), p)))
			}
			if len(matches) == 0 {
				return 0, fmt.Errorf("missing split page %d", p)
			}
			splitPagePath = matches[0]
		}

		// 1. Blank canvas of exact output pageW x pageH
		canvasPath := filepath.Join(tmpDir, fmt.Sprintf("canvas_%d.pdf", p))
		if wErr := os.WriteFile(canvasPath, buildSinglePagePDF(int(math.Round(pageW)), int(math.Round(pageH)), ""), 0644); wErr != nil {
			return 0, wErr
		}

		// 2. Place original page as Form XObject inside exact reserved content rectangle
		composedPath := filepath.Join(tmpDir, fmt.Sprintf("composed_%d.pdf", p))
		placeDesc := fmt.Sprintf("pos:bl, offset:%.2f %.2f, scale:%.4f abs, rot:0, op:1.0", offsetX, offsetY, scale)
		placeWM, pErr := api.PDFWatermark(ctx, splitPagePath, placeDesc, true, false, types.POINTS, newPDFCPUConfig())
		if pErr != nil {
			return 0, fmt.Errorf("failed to prepare manuscript page %d: %w", p, pErr)
		}
		if sErr := api.AddWatermarksFile(ctx, canvasPath, composedPath, nil, placeWM, newPDFCPUConfig()); sErr != nil {
			return 0, fmt.Errorf("failed to compose manuscript page %d: %w", p, sErr)
		}

		currentPage := composedPath

		// 3. Apply independent diagonal 45-degree watermark if enabled
		if opts.WatermarkEnabled && strings.TrimSpace(opts.WatermarkText) != "" {
			wmPath := filepath.Join(tmpDir, fmt.Sprintf("wm_%d.pdf", p))
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
			if wm, wErr := api.TextWatermark(ctx, opts.WatermarkText, wmDesc, false, false, types.POINTS, newPDFCPUConfig()); wErr == nil {
				if sErr := api.AddWatermarksFile(ctx, currentPage, wmPath, nil, wm, newPDFCPUConfig()); sErr == nil {
					currentPage = wmPath
				}
			}
		}

		// 4. Stamp strictly horizontal (rot:0) Header + Footer + Logo + QR + Page X of Y
		stampPDFPath := filepath.Join(tmpDir, fmt.Sprintf("stamp_%d.pdf", p))
		stampBytes := buildPageOverlayPDF(int(math.Round(pageW)), int(math.Round(pageH)), p, pageCount, opts)
		if wErr := os.WriteFile(stampPDFPath, stampBytes, 0644); wErr != nil {
			return 0, wErr
		}
		finalPagePath := filepath.Join(tmpDir, fmt.Sprintf("final_%d.pdf", p))
		overlayWM, oErr := api.PDFWatermark(ctx, stampPDFPath, "rot:0, pos:c, scale:1 abs, op:1.0", true, false, types.POINTS, newPDFCPUConfig())
		if oErr != nil {
			return 0, fmt.Errorf("failed to prepare page %d overlay: %w", p, oErr)
		}
		if sErr := api.AddWatermarksFile(ctx, currentPage, finalPagePath, nil, overlayWM, newPDFCPUConfig()); sErr != nil {
			return 0, fmt.Errorf("failed to stamp page %d: %w", p, sErr)
		}
		brandedPages = append(brandedPages, finalPagePath)
	}

	if err := os.MkdirAll(filepath.Dir(outputPDFPath), 0755); err != nil {
		return 0, err
	}
	if len(brandedPages) == 1 {
		raw, rErr := os.ReadFile(brandedPages[0])
		if rErr != nil {
			return 0, rErr
		}
		if wErr := os.WriteFile(outputPDFPath, raw, 0644); wErr != nil {
			return 0, wErr
		}
	} else {
		if mErr := api.MergeCreateFile(ctx, brandedPages, outputPDFPath, false, newPDFCPUConfig()); mErr != nil {
			return 0, fmt.Errorf("failed to merge branded pages: %w", mErr)
		}
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

	// Header (top 62pt, strictly 0° horizontal inside isolated q/Q graphics state)
	if opts.HeaderEnabled {
		s.WriteString("q\n")
		topY := ph - 20
		textX := 36
		if hasLogo {
			s.WriteString(fmt.Sprintf("q %.2f 0 0 %.2f 36 %d cm /Logo Do Q\n", logoW, logoH, ph-42))
			textX = 36 + int(logoW) + 10
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
			mx := pw - 36 - int(float64(len(meta))*4.2)
			if mx < textX+120 {
				mx = textX + 120
			}
			s.WriteString(fmt.Sprintf("BT /F1 8 Tf 0.28 0.34 0.42 rg %d %d Td (%s) Tj ET\n", mx, topY, escapePDFText(meta)))
		}
		// Navy & Gold separator lines
		s.WriteString(fmt.Sprintf("0.043 0.122 0.227 RG 1 w 36 %d m %d %d l S\n", topY-26, pw-36, topY-26))
		s.WriteString(fmt.Sprintf("0.78 0.60 0.18 RG 0.8 w 36 %d m %d %d l S\n", topY-28, pw-36, topY-28))
		s.WriteString("Q\n")
	}

	// Footer (bottom 54pt, strictly 0° horizontal inside isolated q/Q graphics state)
	if opts.FooterEnabled {
		s.WriteString("q\n")
		s.WriteString(fmt.Sprintf("0.82 0.85 0.90 RG 0.6 w 36 52 m %d 52 l S\n", pw-36))
		s.WriteString("BT /F2 7.5 Tf 0.043 0.122 0.227 rg 36 38 Td (Hosted on ResearchHub | https://researchhub.buzz) Tj ET\n")
		if opts.Disclaimer != "" {
			s.WriteString(fmt.Sprintf("BT /F1 6.5 Tf 0.40 0.45 0.52 rg 36 24 Td (%s) Tj ET\n", escapePDFText(opts.Disclaimer)))
		}
		if opts.PublicationCode != "" {
			pubInfo := fmt.Sprintf("ID: %s", opts.PublicationCode)
			cx := pw/2 - len(pubInfo)*2
			s.WriteString(fmt.Sprintf("BT /F2 7.5 Tf 0.043 0.122 0.227 rg %d 38 Td (%s) Tj ET\n", cx, escapePDFText(pubInfo)))
		}
		if opts.PublicationDate != "" {
			dx := pw/2 - len(opts.PublicationDate)*2
			s.WriteString(fmt.Sprintf("BT /F1 6.5 Tf 0.40 0.45 0.52 rg %d 24 Td (%s) Tj ET\n", dx, escapePDFText(opts.PublicationDate)))
		}
		if opts.PageNumbers {
			pgText := fmt.Sprintf("Page %d of %d", pageNum, totalPages)
			px := pw - 86 - int(float64(len(pgText))*4.2)
			s.WriteString(fmt.Sprintf("BT /F2 7.5 Tf 0.043 0.122 0.227 rg %d 32 Td (%s) Tj ET\n", px, escapePDFText(pgText)))
		}
		if opts.QREnabled && opts.VerificationURL != "" {
			if qr, qErr := qrcode.New(opts.VerificationURL, qrcode.Medium); qErr == nil {
				bm := qr.Bitmap()
				n := len(bm)
				if n > 0 {
					qrSize := 34.0
					ms := qrSize / float64(n)
					ox := float64(pw - 72)
					oy := 14.0
					// White quiet-zone background
					s.WriteString(fmt.Sprintf("1 1 1 rg %.2f %.2f %.2f %.2f re f\n", ox-2, oy-2, qrSize+4, qrSize+4))
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
		s.WriteString("Q\n")
	}

	if hasLogo {
		return buildSinglePagePDFWithLogo(pw, ph, s.String(), opts.LogoPath)
	}
	return buildSinglePagePDF(pw, ph, s.String())
}
