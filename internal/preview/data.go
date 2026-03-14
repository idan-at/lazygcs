package preview

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/hamba/avro/v2/ocf"
	"github.com/parquet-go/parquet-go"
)

// DataPreviewer ...
type DataPreviewer struct{}

// Priority ...
func (p *DataPreviewer) Priority() int { return 20 }

// CanPreview ...
func (p *DataPreviewer) CanPreview(obj Object) bool {
	ext := strings.ToLower(filepath.Ext(obj.Name))
	return ext == ".csv" || ext == ".tsv" || ext == ".parquet" || ext == ".avro" ||
		obj.ContentType == "text/csv" || obj.ContentType == "application/x-parquet"
}

// Preview ...
func (p *DataPreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	ext := strings.ToLower(filepath.Ext(obj.Name))

	switch {
	case ext == ".parquet" || obj.ContentType == "application/x-parquet":
		return p.previewParquet(ctx, client, obj)
	case ext == ".avro":
		return p.previewAvro(ctx, client, obj)
	default:
		return p.previewCSV(ctx, client, obj)
	}
}

func (p *DataPreviewer) previewCSV(ctx context.Context, client GCSClient, obj Object) (string, error) {
	rc, err := client.NewReader(ctx, obj.Bucket, obj.Name)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	reader := csv.NewReader(io.LimitReader(rc, 50*1024)) // Limit to 50KB
	if strings.ToLower(filepath.Ext(obj.Name)) == ".tsv" {
		reader.Comma = '\t'
	}

	rows, err := reader.ReadAll()
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	if len(rows) == 0 {
		return "(empty file)", nil
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240"))).
		Headers(rows[0]...)

	maxRows := 20
	for i := 1; i < len(rows) && i <= maxRows; i++ {
		t.Row(rows[i]...)
	}

	return t.String(), nil
}

func (p *DataPreviewer) previewParquet(ctx context.Context, client GCSClient, obj Object) (string, error) {
	ra := client.NewReaderAt(ctx, obj.Bucket, obj.Name)
	file, err := parquet.OpenFile(ra, obj.Size)
	if err != nil {
		return "", fmt.Errorf("failed to open parquet: %w", err)
	}

	var sb strings.Builder
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)

	fmt.Fprintf(&sb, "%s\n%v\n\n", headerStyle.Render("Parquet Schema:"), file.Schema())
	fmt.Fprintf(&sb, "%s\n", headerStyle.Render(fmt.Sprintf("Rows: %d", file.NumRows())))

	reader := parquet.NewReader(file)
	defer func() { _ = reader.Close() }()

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240")))

	leafColumns := file.Schema().Fields()
	var headers []string
	for _, col := range leafColumns {
		headers = append(headers, col.Name())
	}
	t.Headers(headers...)

	maxRows := 10
	rowsBuf := make([]parquet.Row, maxRows)
	numRowsRead, _ := reader.ReadRows(rowsBuf)

	for i := 0; i < numRowsRead; i++ {
		row := rowsBuf[i]
		var values []string
		for _, v := range row {
			values = append(values, fmt.Sprint(v))
		}
		t.Row(values...)
	}

	sb.WriteString(t.String())
	return sb.String(), nil
}

func (p *DataPreviewer) previewAvro(ctx context.Context, client GCSClient, obj Object) (string, error) {
	rc, err := client.NewReader(ctx, obj.Bucket, obj.Name)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	dec, err := ocf.NewDecoder(rc)
	if err != nil {
		return "", fmt.Errorf("failed to open avro: %w", err)
	}

	var sb strings.Builder
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)

	fmt.Fprintf(&sb, "%s\n%s\n\n", headerStyle.Render("Avro Schema:"), dec.Schema().String())

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240")))

	maxRows := 10
	count := 0
	var headers []string
	for dec.HasNext() && count < maxRows {
		var record map[string]any
		if err := dec.Decode(&record); err != nil {
			break
		}

		if count == 0 {
			for k := range record {
				headers = append(headers, k)
			}
			sort.Strings(headers)
			t.Headers(headers...)
		}

		var values []string
		for _, h := range headers {
			values = append(values, fmt.Sprint(record[h]))
		}
		t.Row(values...)
		count++
	}

	sb.WriteString(t.String())
	return sb.String(), nil
}

// SetWidth ...
func (p *DataPreviewer) SetWidth(_ int) {}
