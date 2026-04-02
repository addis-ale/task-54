package service

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"

	"clinic-admin-suite/internal/repository"
)

type ReportService struct {
	db         *sql.DB
	payments   repository.PaymentRepository
	sharedRoot string
	jobRuns    *JobRunService
	logs       *StructuredLogService
	audit      *AuditService
}

func NewReportService(db *sql.DB, payments repository.PaymentRepository) *ReportService {
	return &ReportService{db: db, payments: payments, sharedRoot: "./data/shared_reports"}
}

func (s *ReportService) ConfigureScheduling(sharedRoot string, jobRuns *JobRunService, logs *StructuredLogService, audit *AuditService) {
	if strings.TrimSpace(sharedRoot) != "" {
		s.sharedRoot = strings.TrimSpace(sharedRoot)
	}
	s.jobRuns = jobRuns
	s.logs = logs
	s.audit = audit
}

type ExportFinanceInput struct {
	Format  string
	Status  string
	Method  string
	Gateway string
	ShiftID string
}

type ReportFile struct {
	FileName    string
	ContentType string
	Body        []byte
}

type OpsSummary struct {
	GeneratedAt           time.Time `json:"generated_at"`
	ActiveAdmissions      int64     `json:"active_admissions"`
	OccupiedBeds          int64     `json:"occupied_beds"`
	OpenWorkOrders        int64     `json:"open_work_orders"`
	PaymentsTodayCount    int64     `json:"payments_today_count"`
	PaymentsTodayNetCents int64     `json:"payments_today_net_cents"`
	FailedPaymentsToday   int64     `json:"failed_payments_today"`
	UpcomingExams48h      int64     `json:"upcoming_exams_48h"`
	OnTimePct             float64   `json:"on_time_pct"`
}

func (s *ReportService) OpsSummary(ctx context.Context) (*OpsSummary, error) {
	if s.db == nil {
		return nil, fmt.Errorf("%w: reporting database is not configured", ErrValidation)
	}

	now := time.Now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	windowEnd := now.Add(48 * time.Hour)

	summary := &OpsSummary{GeneratedAt: now}

	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM admissions WHERE status = 'admitted'`).Scan(&summary.ActiveAdmissions); err != nil {
		return nil, fmt.Errorf("query active admissions: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM beds WHERE status = 'occupied'`).Scan(&summary.OccupiedBeds); err != nil {
		return nil, fmt.Errorf("query occupied beds: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM work_orders WHERE status IN ('open','in_progress')`).Scan(&summary.OpenWorkOrders); err != nil {
		return nil, fmt.Errorf("query open work orders: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM payments WHERE received_at >= ?`, dayStart.Unix()).Scan(&summary.PaymentsTodayCount); err != nil {
		return nil, fmt.Errorf("query payment count: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cents),0) FROM payments WHERE status = 'succeeded' AND received_at >= ?`, dayStart.Unix()).Scan(&summary.PaymentsTodayNetCents); err != nil {
		return nil, fmt.Errorf("query payment net: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM payments WHERE status = 'failed' AND received_at >= ?`, dayStart.Unix()).Scan(&summary.FailedPaymentsToday); err != nil {
		return nil, fmt.Errorf("query failed payments: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM exam_schedules WHERE start_at >= ? AND start_at <= ?`, now.Unix(), windowEnd.Unix()).Scan(&summary.UpcomingExams48h); err != nil {
		return nil, fmt.Errorf("query upcoming exams: %w", err)
	}

	var completedCount, onTimeCount int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM work_orders WHERE status = 'completed'`).Scan(&completedCount); err != nil {
		return nil, fmt.Errorf("query completed work orders: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM work_orders WHERE status = 'completed' AND started_at <= (scheduled_start + 900)`).Scan(&onTimeCount); err != nil {
		return nil, fmt.Errorf("query on-time work orders: %w", err)
	}
	if completedCount > 0 {
		summary.OnTimePct = float64(onTimeCount) * 100 / float64(completedCount)
	}

	return summary, nil
}

func (s *ReportService) ExportFinance(ctx context.Context, input ExportFinanceInput) (*ReportFile, error) {
	format := strings.ToLower(strings.TrimSpace(input.Format))
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "xlsx" {
		return nil, fmt.Errorf("%w: format must be csv or xlsx", ErrValidation)
	}

	rows, err := s.payments.List(ctx, repository.PaymentFilter{
		Status:  strings.TrimSpace(input.Status),
		Method:  strings.TrimSpace(input.Method),
		Gateway: strings.TrimSpace(input.Gateway),
		ShiftID: strings.TrimSpace(input.ShiftID),
	})
	if err != nil {
		return nil, err
	}

	headers := []string{"payment_id", "received_at", "status", "method", "gateway", "amount_cents", "currency", "shift_id", "failure_reason"}
	data := make([][]string, 0, len(rows))
	for _, item := range rows {
		failure := ""
		if item.FailureReason != nil {
			failure = *item.FailureReason
		}
		data = append(data, []string{
			strconv.FormatInt(item.ID, 10),
			item.ReceivedAt.UTC().Format(time.RFC3339),
			item.Status,
			item.Method,
			item.Gateway,
			strconv.FormatInt(item.AmountCents, 10),
			item.Currency,
			item.ShiftID,
			failure,
		})
	}

	timestamp := time.Now().UTC().Format("20060102_150405")
	if format == "csv" {
		var buf bytes.Buffer
		writer := csv.NewWriter(&buf)
		if err := writer.Write(headers); err != nil {
			return nil, fmt.Errorf("write csv header: %w", err)
		}
		if err := writer.WriteAll(data); err != nil {
			return nil, fmt.Errorf("write csv rows: %w", err)
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			return nil, fmt.Errorf("finalize csv: %w", err)
		}
		return &ReportFile{
			FileName:    fmt.Sprintf("finance_report_%s.csv", timestamp),
			ContentType: "text/csv",
			Body:        buf.Bytes(),
		}, nil
	}

	body, err := buildXLSX(headers, data)
	if err != nil {
		return nil, err
	}
	return &ReportFile{
		FileName:    fmt.Sprintf("finance_report_%s.xlsx", timestamp),
		ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		Body:        body,
	}, nil
}

func buildXLSX(headers []string, rows [][]string) ([]byte, error) {
	var out bytes.Buffer
	zipWriter := zip.NewWriter(&out)

	if err := addZipFile(zipWriter, "[Content_Types].xml", contentTypesXML); err != nil {
		return nil, err
	}
	if err := addZipFile(zipWriter, "_rels/.rels", relsXML); err != nil {
		return nil, err
	}
	if err := addZipFile(zipWriter, "xl/workbook.xml", workbookXML); err != nil {
		return nil, err
	}
	if err := addZipFile(zipWriter, "xl/_rels/workbook.xml.rels", workbookRelsXML); err != nil {
		return nil, err
	}
	if err := addZipFile(zipWriter, "xl/styles.xml", stylesXML); err != nil {
		return nil, err
	}

	sheet, err := buildSheetXML(headers, rows)
	if err != nil {
		return nil, err
	}
	if err := addZipFile(zipWriter, "xl/worksheets/sheet1.xml", sheet); err != nil {
		return nil, err
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("finalize xlsx zip: %w", err)
	}

	return out.Bytes(), nil
}

func addZipFile(zipWriter *zip.Writer, name string, payload string) error {
	entry, err := zipWriter.Create(name)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", name, err)
	}
	if _, err := entry.Write([]byte(payload)); err != nil {
		return fmt.Errorf("write zip entry %s: %w", name, err)
	}
	return nil
}

func buildSheetXML(headers []string, rows [][]string) (string, error) {
	type cell struct {
		Ref   string `xml:"r,attr"`
		Style int    `xml:"s,attr,omitempty"`
		Type  string `xml:"t,attr,omitempty"`
		V     string `xml:"v,omitempty"`
		IS    *struct {
			T string `xml:"t"`
		} `xml:"is,omitempty"`
	}
	type rowXML struct {
		Index int    `xml:"r,attr"`
		Cells []cell `xml:"c"`
	}
	type sheetData struct {
		Rows []rowXML `xml:"row"`
	}
	type worksheet struct {
		XMLName xml.Name  `xml:"worksheet"`
		XMLNS   string    `xml:"xmlns,attr"`
		Data    sheetData `xml:"sheetData"`
	}

	buildCell := func(col, row int, value string, header bool) cell {
		ref := excelCol(col) + strconv.Itoa(row)
		c := cell{Ref: ref, Type: "inlineStr", IS: &struct {
			T string `xml:"t"`
		}{T: value}}
		if header {
			c.Style = 1
		}
		return c
	}

	allRows := make([]rowXML, 0, len(rows)+1)
	headerRow := rowXML{Index: 1, Cells: make([]cell, 0, len(headers))}
	for i, h := range headers {
		headerRow.Cells = append(headerRow.Cells, buildCell(i+1, 1, h, true))
	}
	allRows = append(allRows, headerRow)

	for i, data := range rows {
		r := rowXML{Index: i + 2, Cells: make([]cell, 0, len(headers))}
		for col := 0; col < len(headers); col++ {
			value := ""
			if col < len(data) {
				value = data[col]
			}
			r.Cells = append(r.Cells, buildCell(col+1, i+2, value, false))
		}
		allRows = append(allRows, r)
	}

	w := worksheet{
		XMLNS: "http://schemas.openxmlformats.org/spreadsheetml/2006/main",
		Data:  sheetData{Rows: allRows},
	}

	encoded, err := xml.Marshal(w)
	if err != nil {
		return "", fmt.Errorf("marshal xlsx worksheet: %w", err)
	}

	return xml.Header + string(encoded), nil
}

func excelCol(index int) string {
	if index <= 0 {
		return "A"
	}
	var out []byte
	for index > 0 {
		index--
		out = append([]byte{byte('A' + (index % 26))}, out...)
		index /= 26
	}
	return string(out)
}

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
</Types>`

const relsXML = `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`

const workbookXML = `<?xml version="1.0" encoding="UTF-8"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="Finance" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>`

const workbookRelsXML = `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

const stylesXML = `<?xml version="1.0" encoding="UTF-8"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <fonts count="2">
    <font><sz val="11"/><name val="Calibri"/></font>
    <font><b/><sz val="11"/><name val="Calibri"/></font>
  </fonts>
  <fills count="2">
    <fill><patternFill patternType="none"/></fill>
    <fill><patternFill patternType="gray125"/></fill>
  </fills>
  <borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>
  <cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
  <cellXfs count="2">
    <xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
    <xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/>
  </cellXfs>
</styleSheet>`
